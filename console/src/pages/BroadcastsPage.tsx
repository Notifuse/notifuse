import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Card,
  Row,
  Col,
  Typography,
  Space,
  Tooltip,
  Button,
  Divider,
  Modal,
  Input,
  App,
  Badge,
  Descriptions,
  Progress,
  Popover,
  Alert,
  Popconfirm,
  Pagination,
  Tag
} from 'antd'
import { useParams } from '@tanstack/react-router'
import { broadcastApi, Broadcast, BroadcastVariation } from '../services/api/broadcast'
import { listsApi } from '../services/api/list'
import { taskApi } from '../services/api/task'
import { listSegments } from '../services/api/segment'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCirclePause,
  faCircleCheck,
  faCircleXmark,
  faPenToSquare,
  faTrashCan,
  faCirclePlay,
  faCopy,
  faEye,
  faCircleQuestion,
  faPaperPlane
} from '@fortawesome/free-regular-svg-icons'
import {
  faArrowPointer,
  faBan,
  faChevronDown,
  faChevronUp,
  faSpinner,
  faRefresh
} from '@fortawesome/free-solid-svg-icons'
import React, { useState } from 'react'
import dayjs from '../lib/dayjs'
import { UpsertBroadcastDrawer } from '../components/broadcasts/UpsertBroadcastDrawer'
import { SendOrScheduleModal } from '../components/broadcasts/SendOrScheduleModal'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { BroadcastStats } from '../components/broadcasts/BroadcastStats'
import { List, Workspace } from '../services/api/types'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import { Template } from '../services/api/types'

const { Title, Paragraph, Text } = Typography

// Helper function to calculate remaining test time
const getRemainingTestTime = (broadcast: Broadcast, testResults?: any) => {
  if (
    broadcast.status !== 'testing' ||
    !broadcast.test_settings.enabled ||
    !broadcast.test_settings.test_duration_hours
  ) {
    return null
  }

  // Use test_started_at from testResults if available, otherwise use test_sent_at from broadcast
  const testStartTime = testResults?.test_started_at || broadcast.test_sent_at
  if (!testStartTime) {
    return null
  }

  const startTime = dayjs(testStartTime)
  const endTime = startTime.add(broadcast.test_settings.test_duration_hours, 'hours')
  const now = dayjs()

  if (now.isAfter(endTime)) {
    return null // Don't show anything if expired
  }

  // Use dayjs .to() method for natural time formatting
  return now.to(endTime, true) + ' remaining'
}

// Helper function to get status badge
const getStatusBadge = (broadcast: Broadcast, remainingTime?: string | null) => {
  switch (broadcast.status) {
    case 'draft':
      return <Badge status="default" text="Draft" />
    case 'scheduled':
      return <Badge status="processing" text="Scheduled" />
    case 'sending':
      return <Badge status="processing" text="Sending" />
    case 'paused':
      return (
        <Space size="small">
          <Badge status="warning" text="Paused" />
          {broadcast.pause_reason && (
            <Tooltip title={broadcast.pause_reason}>
              <FontAwesomeIcon
                icon={faCircleQuestion}
                className="text-orange-500 cursor-help"
                style={{ opacity: 0.7 }}
              />
            </Tooltip>
          )}
        </Space>
      )
    case 'sent':
      return <Badge status="success" text="Sent" />
    case 'cancelled':
      return <Badge status="error" text="Cancelled" />
    case 'failed':
      return <Badge status="error" text="Failed" />
    case 'testing':
      return (
        <Space size="small">
          <Badge status="processing" text="A/B Testing" />
          {remainingTime && (
            <Text type="secondary" style={{ fontSize: '12px' }}>
              ({remainingTime})
            </Text>
          )}
        </Space>
      )
    case 'test_completed':
      return <Badge status="success" text="Test Completed" />
    case 'winner_selected':
      return <Badge status="success" text="Winner Selected" />
    default:
      return <Badge status="default" text={broadcast.status} />
  }
}

// Component for rendering a single A/B test variation card
interface VariationCardProps {
  variation: BroadcastVariation
  workspace: Workspace
  colSpan: number
  index: number
  broadcast: Broadcast
  onSelectWinner?: (templateId: string) => void
  testResults?: any
  permissions?: any
  onTestTemplate?: (template: Template) => void
}

const VariationCard: React.FC<VariationCardProps> = ({
  variation,
  workspace,
  colSpan,
  index,
  broadcast,
  onSelectWinner,
  testResults,
  permissions,
  onTestTemplate
}) => {
  const emailProvider = workspace.integrations?.find(
    (i) =>
      i.id ===
      (variation.template?.category === 'marketing'
        ? workspace.settings?.marketing_email_provider_id
        : workspace.settings?.transactional_email_provider_id)
  )?.email_provider

  const templateSender = emailProvider?.senders.find(
    (s) => s.id === variation.template?.email?.sender_id
  )

  // Get test results for this variation
  const variationResult = testResults?.variation_results?.[variation.template_id]
  const isWinner = testResults?.winning_template === variation.template_id
  const isRecommendedWinner = testResults?.recommended_winner === variation.template_id
  const canSelectWinner =
    broadcast.status === 'test_completed' && !broadcast.test_settings.auto_send_winner

  return (
    <Col span={colSpan} key={index}>
      <Card
        size="small"
        title={
          <Space>
            {`Variation ${index + 1}: ${variation.template?.name || 'Untitled'}`}
            {isWinner && <Badge status="success" text="Winner" />}
            {isRecommendedWinner && !isWinner && <Badge status="processing" text="Recommended" />}
          </Space>
        }
        type="inner"
        extra={
          <Space>
            {variation.template ? (
              <TemplatePreviewDrawer record={variation.template as any} workspace={workspace}>
                <Button size="small" type="primary" ghost>
                  Preview
                </Button>
              </TemplatePreviewDrawer>
            ) : (
              <Button size="small" type="primary" ghost disabled>
                Preview
              </Button>
            )}
            {variation.template && onTestTemplate && (
              <Tooltip
                title={
                  !(permissions?.templates?.read && permissions?.contacts?.write)
                    ? 'You need read template and write contact permissions to send test emails'
                    : 'Send Test Email'
                }
              >
                <Button
                  size="small"
                  type="text"
                  icon={<FontAwesomeIcon icon={faPaperPlane} />}
                  onClick={() => onTestTemplate(variation.template as Template)}
                  disabled={!(permissions?.templates?.read && permissions?.contacts?.write)}
                />
              </Tooltip>
            )}
            {canSelectWinner && variation.template_id && onSelectWinner && (
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? "You don't have write permission for broadcasts"
                    : undefined
                }
              >
                <Popconfirm
                  title="Select Winner"
                  description={`Are you sure you want to select "${variation.template?.name || 'this variation'}" as the winner? The broadcast will be sent to the remaining recipients.`}
                  onConfirm={() => onSelectWinner(variation.template_id)}
                  okText="Yes, Select Winner"
                  cancelText="Cancel"
                >
                  <Button size="small" type="primary" disabled={!permissions?.broadcasts?.write}>
                    Select Winner
                  </Button>
                </Popconfirm>
              </Tooltip>
            )}
          </Space>
        }
      >
        <Space direction="vertical" size="small">
          <Space>
            <Text strong>From:</Text>
            {templateSender ? (
              <>
                {templateSender.name} &lt;{templateSender.email}&gt;
              </>
            ) : (
              <Text>Default sender</Text>
            )}
          </Space>
          <Space>
            <Text strong>Subject:</Text>
            {variation.template?.email.subject || 'N/A'}
          </Space>
          {variation.template?.email.subject_preview && (
            <Space>
              <Text strong>Subject Preview:</Text>
              {variation.template?.email?.subject_preview}
            </Space>
          )}
          {variation.template?.email?.reply_to && (
            <Text>Reply-to: {variation.template?.email?.reply_to}</Text>
          )}

          {(variation.metrics || variationResult) && (
            <>
              <Divider style={{ margin: '8px 0' }} />
              <div className="grid grid-cols-3 gap-2 mt-2">
                <div>
                  <div className="font-medium text-purple-500 flex items-center">
                    <FontAwesomeIcon icon={faEye} className="mr-1" style={{ opacity: 0.7 }} /> Opens
                  </div>
                  <div>
                    {variationResult ? (
                      <Tooltip
                        title={`${variationResult.opens} opens out of ${variationResult.recipients} recipients`}
                      >
                        <span className="cursor-help">
                          {(variationResult.open_rate * 100).toFixed(1)}%
                        </span>
                      </Tooltip>
                    ) : variation.metrics ? (
                      <Tooltip
                        title={`${variation.metrics.opens} opens out of ${variation.metrics.recipients} recipients`}
                      >
                        <span className="cursor-help">
                          {(variation.metrics.open_rate * 100).toFixed(1)}%
                        </span>
                      </Tooltip>
                    ) : (
                      <Tooltip title="0 opens out of 0 recipients">
                        <span className="cursor-help">0.0%</span>
                      </Tooltip>
                    )}
                  </div>
                </div>
                <div>
                  <div className="font-medium text-cyan-500 flex items-center">
                    <FontAwesomeIcon
                      icon={faArrowPointer}
                      className="mr-1"
                      style={{ opacity: 0.7 }}
                    />{' '}
                    Clicks
                  </div>
                  <div>
                    {variationResult ? (
                      <Tooltip
                        title={`${variationResult.clicks} clicks out of ${variationResult.recipients} recipients`}
                      >
                        <span className="cursor-help">
                          {(variationResult.click_rate * 100).toFixed(1)}%
                        </span>
                      </Tooltip>
                    ) : variation.metrics ? (
                      <Tooltip
                        title={`${variation.metrics.clicks} clicks out of ${variation.metrics.recipients} recipients`}
                      >
                        <span className="cursor-help">
                          {(variation.metrics.click_rate * 100).toFixed(1)}%
                        </span>
                      </Tooltip>
                    ) : (
                      <Tooltip title="0 clicks out of 0 recipients">
                        <span className="cursor-help">0.0%</span>
                      </Tooltip>
                    )}
                  </div>
                </div>
                <div>
                  <div className="font-medium text-green-500 flex items-center">
                    <FontAwesomeIcon
                      icon={faCircleCheck}
                      className="mr-1"
                      style={{ opacity: 0.7 }}
                    />{' '}
                    Delivered
                  </div>
                  <div>
                    {variationResult ? (
                      <Tooltip
                        title={`${variationResult.delivered || 0} successfully delivered out of ${variationResult.recipients || 0} total recipients`}
                      >
                        <span className="cursor-help">
                          {variationResult.recipients && variationResult.recipients > 0
                            ? (
                                (variationResult.delivered / variationResult.recipients) *
                                100
                              ).toFixed(1)
                            : '0.0'}
                          %
                        </span>
                      </Tooltip>
                    ) : variation.metrics ? (
                      <Tooltip
                        title={`${variation.metrics.delivered || 0} successfully delivered out of ${variation.metrics.recipients || 0} total recipients`}
                      >
                        <span className="cursor-help">
                          {variation.metrics.recipients && variation.metrics.recipients > 0
                            ? (
                                (variation.metrics.delivered / variation.metrics.recipients) *
                                100
                              ).toFixed(1)
                            : '0.0'}
                          %
                        </span>
                      </Tooltip>
                    ) : (
                      <Tooltip title="0 delivered out of 0 recipients">
                        <span className="cursor-help">0.0%</span>
                      </Tooltip>
                    )}
                  </div>
                </div>
              </div>
            </>
          )}
        </Space>
      </Card>
    </Col>
  )
}

// Component for rendering a single broadcast card
interface BroadcastCardProps {
  broadcast: Broadcast
  lists: List[]
  segments: { id: string; name: string; color: string; users_count?: number }[]
  workspaceId: string
  onDelete: (broadcast: Broadcast) => void
  onPause: (broadcast: Broadcast) => void
  onResume: (broadcast: Broadcast) => void
  onCancel: (broadcast: Broadcast) => void
  onSchedule: (broadcast: Broadcast) => void
  onRefresh: (broadcast: Broadcast) => void
  currentWorkspace: any
  permissions: any
  isFirst?: boolean
  currentPage: number
  pageSize: number
}

const BroadcastCard: React.FC<BroadcastCardProps> = ({
  broadcast,
  lists,
  segments,
  workspaceId,
  onDelete,
  onPause,
  onResume,
  onCancel,
  onSchedule,
  onRefresh,
  currentWorkspace,
  permissions,
  isFirst = false,
  currentPage,
  pageSize
}) => {
  const [showDetails, setShowDetails] = useState(isFirst)
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [testModalOpen, setTestModalOpen] = useState(false)
  const [templateToTest, setTemplateToTest] = useState<Template | null>(null)

  // Fetch task associated with this broadcast
  const { data: task, isLoading: isTaskLoading } = useQuery({
    queryKey: ['task', workspaceId, broadcast.id],
    queryFn: () => {
      return taskApi.findByBroadcastId(workspaceId, broadcast.id)
    },
    // Only fetch task data if the broadcast status indicates a task might exist
    // enabled: ['scheduled', 'sending', 'paused', 'failed'].includes(broadcast.status),
    refetchInterval:
      broadcast.status === 'sending'
        ? 5000 // Refetch every 5 seconds for sending broadcasts
        : broadcast.status === 'scheduled'
          ? 30000 // Refetch every 30 seconds for scheduled broadcasts
          : false // Don't auto-refetch for other statuses
  })

  // Fetch test results if broadcast has A/B testing enabled and is in testing phase
  const { data: testResults } = useQuery({
    queryKey: ['testResults', workspaceId, broadcast.id],
    queryFn: () => {
      return broadcastApi.getTestResults({
        workspace_id: workspaceId,
        id: broadcast.id
      })
    },
    enabled:
      broadcast.test_settings.enabled &&
      ['testing', 'test_completed', 'winner_selected'].includes(broadcast.status),
    refetchInterval: broadcast.status === 'testing' ? 10000 : false // Refetch every 10 seconds during testing
  })

  // Calculate remaining test time
  const remainingTestTime = getRemainingTestTime(broadcast, testResults)

  // Handler for selecting winner
  const handleSelectWinner = async (templateId: string) => {
    try {
      await broadcastApi.selectWinner({
        workspace_id: workspaceId,
        id: broadcast.id,
        template_id: templateId
      })
      message.success(
        'Winner selected successfully! The broadcast will be sent to remaining recipients.'
      )
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
      queryClient.invalidateQueries({ queryKey: ['testResults', workspaceId, broadcast.id] })
    } catch (error) {
      message.error('Failed to select winner')
      console.error(error)
    }
  }

  // Handler for testing a template
  const handleTestTemplate = (template: Template) => {
    setTemplateToTest(template)
    setTestModalOpen(true)
  }

  // Helper function to render task status badge
  const getTaskStatusBadge = (status: string) => {
    switch (status) {
      case 'pending':
        return <Badge status="processing" text="Pending" />
      case 'running':
        return <Badge status="processing" text="Running" />
      case 'completed':
        return <Badge status="success" text="Completed" />
      case 'failed':
        return <Badge status="error" text="Failed" />
      case 'cancelled':
        return <Badge status="warning" text="Cancelled" />
      case 'paused':
        return <Badge status="warning" text="Paused" />
      default:
        return <Badge status="default" text={status} />
    }
  }

  // Create popover content for details
  const taskPopoverContent = () => {
    if (!task) return null

    return (
      <div className="max-w-xs">
        <div className="mb-2">
          <div className="font-medium text-gray-500">Status</div>
          <div>{getTaskStatusBadge(task.status)}</div>
        </div>

        {task.next_run_after && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">Next Run</div>
            <div className="text-sm">
              {task.status === 'paused' ? (
                <Tooltip title={dayjs(task.next_run_after).format('lll')}>
                  <span className="text-orange-600">{dayjs(task.next_run_after).fromNow()}</span>
                </Tooltip>
              ) : task.status === 'pending' ? (
                <Tooltip title={dayjs(task.next_run_after).format('lll')}>
                  <span className="text-blue-600">{dayjs(task.next_run_after).fromNow()}</span>
                </Tooltip>
              ) : (
                <Tooltip title={dayjs(task.next_run_after).format('lll')}>
                  <span>{dayjs(task.next_run_after).fromNow()}</span>
                </Tooltip>
              )}
            </div>
          </div>
        )}

        {(task.progress > 0 || task.state?.send_broadcast) && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">Progress</div>
            <Progress
              percent={Math.round(
                task.state?.send_broadcast
                  ? (task.state.send_broadcast.sent_count /
                      task.state.send_broadcast.total_recipients) *
                      100
                  : task.progress * 100
              )}
              size="small"
            />
          </div>
        )}

        {task.state?.message && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">Message</div>
            <div>{task.state.message}</div>
          </div>
        )}

        {task.state?.send_broadcast && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">Broadcast Progress</div>
            <div className="text-sm">
              Sent: {task.state.send_broadcast.sent_count} of{' '}
              {task.state.send_broadcast.total_recipients}
              {task.state.send_broadcast.failed_count > 0 && (
                <div className="text-red-500">Failed: {task.state.send_broadcast.failed_count}</div>
              )}
            </div>
          </div>
        )}

        {task.error_message && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">Error</div>
            <div className="text-red-500 text-sm">{task.error_message}</div>
          </div>
        )}

        {task.type && <div className="text-xs text-gray-500 mt-2">Task type: {task.type}</div>}
      </div>
    )
  }

  return (
    <Card
      styles={{
        body: {
          padding: 0
        }
      }}
      title={
        <Space size="large">
          <div>{broadcast.name}</div>
          <div className="text-xs font-normal">
            {task ? (
              <Popover
                content={taskPopoverContent}
                title="Task Status"
                placement="bottom"
                trigger="hover"
              >
                <span className="cursor-help">
                  {getStatusBadge(broadcast, remainingTestTime)}
                  <FontAwesomeIcon
                    icon={faCircleQuestion}
                    style={{ opacity: 0.7 }}
                    className="ml-2"
                  />
                </span>
              </Popover>
            ) : isTaskLoading ? (
              <span className="text-gray-400">
                {getStatusBadge(broadcast, remainingTestTime)}
                <FontAwesomeIcon icon={faSpinner} spin className="ml-2" />
              </span>
            ) : (
              getStatusBadge(broadcast, remainingTestTime)
            )}
          </div>
        </Space>
      }
      extra={
        <Space>
          <Tooltip title="Refresh Broadcast">
            <Button
              type="text"
              size="small"
              icon={<FontAwesomeIcon icon={faRefresh} />}
              onClick={() => onRefresh(broadcast)}
              className="opacity-70 hover:opacity-100"
            />
          </Tooltip>
          {(broadcast.status === 'draft' || broadcast.status === 'scheduled') && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? "You don't have write permission for broadcasts"
                  : 'Edit Broadcast'
              }
            >
              <div>
                <UpsertBroadcastDrawer
                  workspace={currentWorkspace!}
                  broadcast={broadcast}
                  lists={lists}
                  segments={segments}
                  buttonContent={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                  buttonProps={{
                    size: 'small',
                    type: 'text',
                    disabled: !permissions?.broadcasts?.write
                  }}
                />
              </div>
            </Tooltip>
          )}
          {broadcast.status === 'sending' && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? "You don't have write permission for broadcasts"
                  : 'Pause Broadcast'
              }
            >
              <Popconfirm
                title="Pause broadcast?"
                description="The broadcast will stop sending and can be resumed later."
                onConfirm={() => onPause(broadcast)}
                okText="Yes, pause"
                cancelText="Cancel"
                disabled={!permissions?.broadcasts?.write}
              >
                <Button type="text" size="small" disabled={!permissions?.broadcasts?.write}>
                  <FontAwesomeIcon icon={faCirclePause} style={{ opacity: 0.7 }} />
                </Button>
              </Popconfirm>
            </Tooltip>
          )}
          {broadcast.status === 'paused' && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? "You don't have write permission for broadcasts"
                  : 'Resume Broadcast'
              }
            >
              <Popconfirm
                title="Resume broadcast?"
                description="The broadcast will continue sending from where it was paused."
                onConfirm={() => onResume(broadcast)}
                okText="Yes, resume"
                cancelText="Cancel"
                disabled={!permissions?.broadcasts?.write}
              >
                <Button type="text" size="small" disabled={!permissions?.broadcasts?.write}>
                  <FontAwesomeIcon icon={faCirclePlay} style={{ opacity: 0.7 }} />
                </Button>
              </Popconfirm>
            </Tooltip>
          )}
          {broadcast.status === 'scheduled' && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? "You don't have write permission for broadcasts"
                  : 'Cancel Broadcast'
              }
            >
              <Button
                type="text"
                size="small"
                onClick={() => onCancel(broadcast)}
                disabled={!permissions?.broadcasts?.write}
              >
                <FontAwesomeIcon icon={faBan} style={{ opacity: 0.7 }} />
              </Button>
            </Tooltip>
          )}
          {broadcast.status === 'draft' && (
            <>
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? "You don't have write permission for broadcasts"
                    : 'Delete Broadcast'
                }
              >
                <Button
                  type="text"
                  size="small"
                  onClick={() => onDelete(broadcast)}
                  disabled={!permissions?.broadcasts?.write}
                >
                  <FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />
                </Button>
              </Tooltip>
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? "You don't have write permission for broadcasts"
                    : undefined
                }
              >
                <Button
                  type="primary"
                  size="small"
                  ghost
                  disabled={
                    !permissions?.broadcasts?.write ||
                    !currentWorkspace?.settings?.marketing_email_provider_id
                  }
                  onClick={() => onSchedule(broadcast)}
                >
                  Send or Schedule
                </Button>
              </Tooltip>
            </>
          )}
        </Space>
      }
      key={broadcast.id}
      className="!mb-6"
    >
      <div className="p-6">
        <BroadcastStats workspaceId={workspaceId} broadcastId={broadcast.id} />
      </div>

      <div className="bg-gray-50">
        <div className="text-center py-2">
          <Button type="link" onClick={() => setShowDetails(!showDetails)}>
            {showDetails ? (
              <Space size="small">
                <FontAwesomeIcon icon={faChevronUp} style={{ opacity: 0.7 }} className="mr-1" />{' '}
                Hide Details
              </Space>
            ) : (
              <Space size="small">
                <FontAwesomeIcon icon={faChevronDown} style={{ opacity: 0.7 }} className="mr-1" />{' '}
                Show Details
              </Space>
            )}
          </Button>
        </div>

        {showDetails && (
          <div className="p-6">
            <Row gutter={[24, 16]}>
              {/* Left Column: Descriptions */}
              <Col xs={24} lg={12} xl={10}>
                <Descriptions bordered={false} size="small" column={1}>
                  {broadcast.started_at && (
                    <Descriptions.Item label="Started">
                      {dayjs(broadcast.started_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {broadcast.completed_at && (
                    <Descriptions.Item label="Completed">
                      {dayjs(broadcast.completed_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {broadcast.paused_at && (
                    <Descriptions.Item label="Paused">
                      <Space direction="vertical" size="small">
                        <div>{dayjs(broadcast.paused_at).fromNow()}</div>
                        {broadcast.pause_reason && (
                          <div className="text-orange-600 text-sm">
                            <strong>Reason:</strong> {broadcast.pause_reason}
                          </div>
                        )}
                      </Space>
                    </Descriptions.Item>
                  )}

                  {broadcast.cancelled_at && (
                    <Descriptions.Item label="Cancelled">
                      {dayjs(broadcast.cancelled_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {/* Audience Information */}
                  {broadcast.audience.segments && broadcast.audience.segments.length > 0 && (
                    <Descriptions.Item label="Segments">
                      <Space wrap>
                        {broadcast.audience.segments.map((segmentId) => {
                          const segment = segments.find((s) => s.id === segmentId)
                          return segment ? (
                            <Tag key={segment.id} color={segment.color} bordered={false}>
                              {segment.name}
                            </Tag>
                          ) : (
                            <Tag key={segmentId} bordered={false}>
                              Unknown segment ({segmentId})
                            </Tag>
                          )
                        })}
                      </Space>
                    </Descriptions.Item>
                  )}

                  {broadcast.audience.list && (
                    <Descriptions.Item label="List">
                      {(() => {
                        const list = lists.find((l) => l.id === broadcast.audience.list)
                        return list ? list.name : `Unknown list (${broadcast.audience.list})`
                      })()}
                    </Descriptions.Item>
                  )}

                  <Descriptions.Item label="Exclude Unsubscribed">
                    {broadcast.audience.exclude_unsubscribed ? (
                      <FontAwesomeIcon
                        icon={faCircleCheck}
                        className="text-green-500 opacity-70 mt-1"
                      />
                    ) : (
                      <FontAwesomeIcon
                        icon={faCircleXmark}
                        className="text-orange-500 opacity-70 mt-1"
                      />
                    )}
                  </Descriptions.Item>

                  {broadcast.utm_parameters?.source && (
                    <Descriptions.Item label="UTM Source">
                      {broadcast.utm_parameters.source}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.medium && (
                    <Descriptions.Item label="UTM Medium">
                      {broadcast.utm_parameters.medium}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.campaign && (
                    <Descriptions.Item label="UTM Campaign">
                      {broadcast.utm_parameters.campaign}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.term && (
                    <Descriptions.Item label="UTM Term">
                      {broadcast.utm_parameters.term}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.content && (
                    <Descriptions.Item label="UTM Content">
                      {broadcast.utm_parameters.content}
                    </Descriptions.Item>
                  )}

                  {/* Web Publication Settings */}
                  <Descriptions.Item label="Web Channel">
                    {broadcast.channels?.web ? (
                      <Tag bordered={false} color="green">
                        Enabled
                      </Tag>
                    ) : (
                      <Tag bordered={false} color="volcano">
                        Disabled
                      </Tag>
                    )}
                  </Descriptions.Item>

                  {broadcast.channels?.web && broadcast.web_publication_settings && (
                    <>
                      <Descriptions.Item label="Post URL" span={1}>
                        {broadcast.web_publication_settings.slug &&
                        currentWorkspace?.settings?.custom_endpoint_url ? (
                          <div className="flex items-center gap-2">
                            {(() => {
                              const list = lists.find((l) => l.id === broadcast.audience.list)
                              if (list?.slug) {
                                return (
                                  <a
                                    href={`${currentWorkspace.settings.custom_endpoint_url}/${list.slug}/${broadcast.web_publication_settings.slug}`}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="text-sm"
                                  >
                                    {currentWorkspace.settings.custom_endpoint_url}/{list.slug}/
                                    {broadcast.web_publication_settings.slug}
                                  </a>
                                )
                              }
                              return <Text type="secondary">List slug not configured</Text>
                            })()}
                          </div>
                        ) : (
                          <Text type="secondary">Not set</Text>
                        )}
                      </Descriptions.Item>

                      {broadcast.web_published_at && (
                        <Descriptions.Item label="Published">
                          {dayjs(broadcast.web_published_at).fromNow()}
                        </Descriptions.Item>
                      )}

                      {broadcast.web_publication_settings.meta_title && (
                        <Descriptions.Item label="SEO Title">
                          {broadcast.web_publication_settings.meta_title}
                        </Descriptions.Item>
                      )}

                      {broadcast.web_publication_settings.meta_description && (
                        <Descriptions.Item label="SEO Description">
                          {broadcast.web_publication_settings.meta_description}
                        </Descriptions.Item>
                      )}

                      {broadcast.web_publication_settings.keywords &&
                        broadcast.web_publication_settings.keywords.length > 0 && (
                          <Descriptions.Item label="SEO Keywords">
                            <Space size={4} wrap>
                              {broadcast.web_publication_settings.keywords.map((keyword, idx) => (
                                <Tag key={idx} bordered={false} className="text-xs">
                                  {keyword}
                                </Tag>
                              ))}
                            </Space>
                          </Descriptions.Item>
                        )}
                    </>
                  )}

                  {/* Schedule Information */}
                  {broadcast.schedule.is_scheduled &&
                    broadcast.schedule.scheduled_date &&
                    broadcast.schedule.scheduled_time && (
                      <Descriptions.Item label="Scheduled">
                        {dayjs(
                          `${broadcast.schedule.scheduled_date} ${broadcast.schedule.scheduled_time}`
                        ).format('lll')}
                        {' in '}
                        {broadcast.schedule.use_recipient_timezone
                          ? 'recipients timezone'
                          : broadcast.schedule.timezone}
                      </Descriptions.Item>
                    )}

                  {/* A/B Test Settings */}
                  {!broadcast.test_settings.enabled && (
                    <Descriptions.Item label="Test Sample">
                      <Badge status="warning" text="A/B Test Disabled" />
                    </Descriptions.Item>
                  )}
                  {broadcast.test_settings.enabled && (
                    <Descriptions.Item label="Test Sample">
                      {broadcast.test_settings.sample_percentage}%
                    </Descriptions.Item>
                  )}

                  {broadcast.test_settings.auto_send_winner &&
                    broadcast.test_settings.auto_send_winner_metric &&
                    broadcast.test_settings.test_duration_hours && (
                      <Descriptions.Item label="Auto-send Winner">
                        <div className="flex items-center">
                          <FontAwesomeIcon
                            icon={faCircleCheck}
                            className="text-green-500 mr-2"
                            size="sm"
                            style={{ opacity: 0.7 }}
                          />
                          <span>
                            After {broadcast.test_settings.test_duration_hours} hours based on
                            highest{' '}
                            {broadcast.test_settings.auto_send_winner_metric === 'open_rate'
                              ? 'opens'
                              : 'clicks'}
                          </span>
                        </div>
                      </Descriptions.Item>
                    )}

                  {/* Test Results Summary */}
                  {testResults && testResults.test_started_at && (
                    <Descriptions.Item label="Test Started">
                      {dayjs(testResults.test_started_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {testResults && testResults.test_completed_at && (
                    <Descriptions.Item label="Test Completed">
                      {dayjs(testResults.test_completed_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {testResults && testResults.recommended_winner && (
                    <Descriptions.Item label="Recommended Winner">
                      <Space>
                        <Badge status="processing" text="Recommended" />
                        {Object.values(testResults.variation_results).find(
                          (result) => result.template_id === testResults.recommended_winner
                        )?.template_name || 'Unknown'}
                      </Space>
                    </Descriptions.Item>
                  )}

                  {testResults && testResults.winning_template && (
                    <Descriptions.Item label="Selected Winner">
                      <Space>
                        <Badge status="success" text="Winner" />
                        {Object.values(testResults.variation_results).find(
                          (result) => result.template_id === testResults.winning_template
                        )?.template_name || 'Unknown'}
                      </Space>
                    </Descriptions.Item>
                  )}
                </Descriptions>

                {/* Open Graph Preview - Separate section */}
                {broadcast.channels?.web && broadcast.web_publication_settings && (
                  <Descriptions
                    size="small"
                    layout="vertical"
                    column={1}
                    style={{ marginTop: 16 }}
                  >
                    <Descriptions.Item label="Open Graph Preview">
                      <div
                        className="border border-gray-200 rounded-lg overflow-hidden bg-white flex"
                        style={{ width: 350 }}
                      >
                        {/* OG Image - Square on the left */}
                        {broadcast.web_publication_settings.og_image ? (
                          <div className="w-24 h-24 flex-shrink-0 bg-gray-100 overflow-hidden">
                            <img
                              src={broadcast.web_publication_settings.og_image}
                              alt={
                                broadcast.web_publication_settings.og_title || broadcast.name
                              }
                              className="w-full h-full object-cover"
                            />
                          </div>
                        ) : (
                          <div className="w-24 h-24 flex-shrink-0 bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
                            <FontAwesomeIcon
                              icon={faCircleCheck}
                              className="text-blue-300"
                              size="2x"
                            />
                          </div>
                        )}

                        {/* OG Content - Text on the right */}
                        <div className="flex-1 p-3 flex flex-col justify-center min-w-0">
                          {currentWorkspace?.settings?.custom_endpoint_url && (
                            <div className="text-xs text-gray-500 mb-1 truncate">
                              {currentWorkspace.settings.custom_endpoint_url.replace(
                                /^https?:\/\//,
                                ''
                              )}
                            </div>
                          )}
                          <div className="text-sm font-semibold text-gray-900 mb-1 line-clamp-2">
                            {broadcast.web_publication_settings.og_title ||
                              broadcast.web_publication_settings.meta_title ||
                              broadcast.name}
                          </div>
                          <div className="text-xs text-gray-600 line-clamp-2">
                            {broadcast.web_publication_settings.og_description ||
                              broadcast.web_publication_settings.meta_description ||
                              `Read the latest post from this broadcast.`}
                          </div>
                        </div>
                      </div>
                    </Descriptions.Item>
                  </Descriptions>
                )}
              </Col>

              {/* Right Column: Templates */}
              <Col xs={24} lg={12} xl={14}>
                <Row gutter={[16, 16]}>
                  {broadcast.test_settings.variations.map((variation, index) => {
                    return (
                      <VariationCard
                        key={index}
                        variation={variation}
                        workspace={currentWorkspace}
                        colSpan={24}
                        index={index}
                        broadcast={broadcast}
                        onSelectWinner={handleSelectWinner}
                        testResults={testResults}
                        permissions={permissions}
                        onTestTemplate={handleTestTemplate}
                      />
                    )
                  })}
                </Row>
              </Col>
            </Row>
          </div>
        )}
      </div>

      {/* Test Template Modal */}
      <SendTemplateModal
        isOpen={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        template={templateToTest}
        workspace={currentWorkspace}
      />
    </Card>
  )
}

export function BroadcastsPage() {
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/broadcasts' })
  const [deleteModalVisible, setDeleteModalVisible] = useState(false)
  const [broadcastToDelete, setBroadcastToDelete] = useState<Broadcast | null>(null)
  const [confirmationInput, setConfirmationInput] = useState('')
  const [isDeleting, setIsDeleting] = useState(false)
  const [isScheduleModalVisible, setIsScheduleModalVisible] = useState(false)
  const [broadcastToSchedule, setBroadcastToSchedule] = useState<Broadcast | null>(null)
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize] = useState(5)
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
  const { message } = App.useApp()

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  const { data, isLoading } = useQuery({
    queryKey: ['broadcasts', workspaceId, currentPage, pageSize],
    queryFn: () => {
      return broadcastApi.list({
        workspace_id: workspaceId,
        with_templates: true,
        limit: pageSize,
        offset: (currentPage - 1) * pageSize
      })
    }
  })

  // Fetch lists for the current workspace
  const { data: listsData } = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => {
      return listsApi.list({ workspace_id: workspaceId, with_templates: true })
    }
  })

  const lists = listsData?.lists || []

  // Fetch segments for the current workspace
  const { data: segmentsData } = useQuery({
    queryKey: ['segments', workspaceId],
    queryFn: () => {
      return listSegments({ workspace_id: workspaceId, with_count: true })
    }
  })

  const segments = segmentsData?.segments || []

  const handleDeleteBroadcast = async () => {
    if (!broadcastToDelete) return

    setIsDeleting(true)
    try {
      await broadcastApi.delete({
        workspace_id: workspaceId,
        id: broadcastToDelete.id
      })

      message.success(`Broadcast "${broadcastToDelete.name}" deleted successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })

      // If we're on a page > 1 and this was the last item on the page, go to previous page
      if (currentPage > 1 && data?.broadcasts.length === 1) {
        setCurrentPage(currentPage - 1)
      }
      setDeleteModalVisible(false)
      setBroadcastToDelete(null)
      setConfirmationInput('')
    } catch (error) {
      message.error('Failed to delete broadcast')
      console.error(error)
    } finally {
      setIsDeleting(false)
    }
  }

  const handlePauseBroadcast = async (broadcast: Broadcast) => {
    try {
      await broadcastApi.pause({
        workspace_id: workspaceId,
        id: broadcast.id
      })
      message.success(`Broadcast "${broadcast.name}" paused successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error('Failed to pause broadcast')
      console.error(error)
    }
  }

  const handleResumeBroadcast = async (broadcast: Broadcast) => {
    try {
      await broadcastApi.resume({
        workspace_id: workspaceId,
        id: broadcast.id
      })
      message.success(`Broadcast "${broadcast.name}" resumed successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error('Failed to resume broadcast')
      console.error(error)
    }
  }

  const handleCancelBroadcast = async (broadcast: Broadcast) => {
    try {
      await broadcastApi.cancel({
        workspace_id: workspaceId,
        id: broadcast.id
      })
      message.success(`Broadcast "${broadcast.name}" cancelled successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error('Failed to cancel broadcast')
      console.error(error)
    }
  }

  const openDeleteModal = (broadcast: Broadcast) => {
    setBroadcastToDelete(broadcast)
    setDeleteModalVisible(true)
  }

  const closeDeleteModal = () => {
    setDeleteModalVisible(false)
    setBroadcastToDelete(null)
    setConfirmationInput('')
  }

  const handleScheduleBroadcast = (broadcast: Broadcast) => {
    setBroadcastToSchedule(broadcast)
    setIsScheduleModalVisible(true)
  }

  const closeScheduleModal = () => {
    setIsScheduleModalVisible(false)
    setBroadcastToSchedule(null)
  }

  const handleRefreshBroadcast = (broadcast: Broadcast) => {
    // Refresh specific broadcast data
    queryClient.invalidateQueries({ queryKey: ['broadcast-stats', workspaceId, broadcast.id] })
    queryClient.invalidateQueries({ queryKey: ['task', workspaceId, broadcast.id] })
    queryClient.invalidateQueries({ queryKey: ['testResults', workspaceId, broadcast.id] })
    // Also refresh the main broadcast data to get updated status
    queryClient.invalidateQueries({ queryKey: ['broadcasts', workspaceId, currentPage, pageSize] })
    message.success(`Broadcast "${broadcast.name}" refreshed`)
  }

  const handlePageChange = (page: number) => {
    setCurrentPage(page)
    // Scroll to top when page changes
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const hasBroadcasts = !isLoading && data?.broadcasts && data.broadcasts.length > 0
  const hasMarketingEmailProvider = currentWorkspace?.settings?.marketing_email_provider_id

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Broadcasts</div>
        {currentWorkspace && hasBroadcasts && (
          <Space>
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? "You don't have write permission for broadcasts"
                  : undefined
              }
            >
              <div>
                <UpsertBroadcastDrawer
                  workspace={currentWorkspace}
                  lists={lists}
                  segments={segments}
                  buttonContent={<>Create Broadcast</>}
                  buttonProps={{
                    disabled: !permissions?.broadcasts?.write
                  }}
                />
              </div>
            </Tooltip>
          </Space>
        )}
      </div>

      {!hasMarketingEmailProvider && (
        <Alert
          message="Email Provider Required"
          description="You don't have a marketing email provider configured. Please set up an email provider in your workspace settings to send broadcasts."
          type="warning"
          showIcon
          className="!mb-6"
          action={
            <Button
              type="primary"
              size="small"
              href={`/console/workspace/${workspaceId}/settings/integrations`}
            >
              Configure Provider
            </Button>
          }
        />
      )}

      {isLoading ? (
        <Row gutter={[16, 16]}>
          {[1, 2, 3].map((key) => (
            <Col xs={24} sm={12} lg={8} key={key}>
              <Card loading variant="outlined" />
            </Col>
          ))}
        </Row>
      ) : hasBroadcasts ? (
        <div>
          {data.broadcasts.map((broadcast: Broadcast, index) => (
            <BroadcastCard
              key={broadcast.id}
              broadcast={broadcast}
              lists={lists}
              segments={segments}
              workspaceId={workspaceId}
              onDelete={openDeleteModal}
              onPause={handlePauseBroadcast}
              onResume={handleResumeBroadcast}
              onCancel={handleCancelBroadcast}
              onSchedule={handleScheduleBroadcast}
              onRefresh={handleRefreshBroadcast}
              currentWorkspace={currentWorkspace}
              permissions={permissions}
              isFirst={index === 0}
              currentPage={currentPage}
              pageSize={pageSize}
            />
          ))}

          {/* Pagination */}
          {data && data.total_count > pageSize && (
            <div className="flex justify-center mt-8">
              <Pagination
                current={currentPage}
                pageSize={pageSize}
                total={data.total_count}
                onChange={handlePageChange}
                showSizeChanger={false}
                showQuickJumper={false}
                showTotal={(total, range) => `${range[0]}-${range[1]} of ${total} broadcasts`}
              />
            </div>
          )}
        </div>
      ) : (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            No broadcasts found
          </Title>
          <Paragraph type="secondary">Create your first broadcast to get started</Paragraph>
          <div className="mt-4">
            {currentWorkspace && (
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? "You don't have write permission for broadcasts"
                    : undefined
                }
              >
                <div>
                  <UpsertBroadcastDrawer
                    workspace={currentWorkspace}
                    lists={lists}
                    segments={segments}
                    buttonContent="Create Broadcast"
                    buttonProps={{
                      disabled: !permissions?.broadcasts?.write
                    }}
                  />
                </div>
              </Tooltip>
            )}
          </div>
        </div>
      )}

      <SendOrScheduleModal
        broadcast={broadcastToSchedule}
        visible={isScheduleModalVisible}
        onClose={closeScheduleModal}
        workspaceId={workspaceId}
        workspace={currentWorkspace}
        onSuccess={() => {
          queryClient.invalidateQueries({
            queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
          })
        }}
      />

      <Modal
        title="Delete Broadcast"
        open={deleteModalVisible}
        onCancel={closeDeleteModal}
        footer={[
          <Button key="cancel" onClick={closeDeleteModal}>
            Cancel
          </Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={isDeleting}
            disabled={confirmationInput !== (broadcastToDelete?.id || '')}
            onClick={handleDeleteBroadcast}
          >
            Delete
          </Button>
        ]}
      >
        {broadcastToDelete && (
          <>
            <p>Are you sure you want to delete the broadcast "{broadcastToDelete.name}"?</p>
            <p>
              This action cannot be undone. To confirm, please enter the broadcast ID:{' '}
              <Text code>{broadcastToDelete.id}</Text>
              <Tooltip title="Copy to clipboard">
                <Button
                  type="text"
                  icon={<FontAwesomeIcon icon={faCopy} style={{ opacity: 0.7 }} />}
                  size="small"
                  onClick={() => {
                    navigator.clipboard.writeText(broadcastToDelete.id)
                    message.success('Broadcast ID copied to clipboard')
                  }}
                />
              </Tooltip>
            </p>
            <Input
              placeholder="Enter broadcast ID to confirm"
              value={confirmationInput}
              onChange={(e) => setConfirmationInput(e.target.value)}
              status={
                confirmationInput && confirmationInput !== broadcastToDelete.id ? 'error' : ''
              }
            />
            {confirmationInput && confirmationInput !== broadcastToDelete.id && (
              <p className="text-red-500 mt-2">ID doesn't match</p>
            )}
          </>
        )}
      </Modal>
    </div>
  )
}
