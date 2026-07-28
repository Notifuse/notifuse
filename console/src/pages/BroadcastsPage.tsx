import { useQuery, useQueries, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import {
  Card,
  Row,
  Col,
  Typography,
  Space,
  Tooltip,
  Button,
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
  Tag,
  Table,
  Segmented
} from 'antd'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import {
  broadcastApi,
  Broadcast,
  TestResultsResponse,
  getStatusesForGroup
} from '../services/api/broadcast'
import { listsApi } from '../services/api/list'
import { taskApi } from '../services/api/task'
import {
  getBroadcastVariationStats,
  MessageHistoryStatusSum
} from '../services/api/messages_history'
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
import React, { useState, useEffect, useRef } from 'react'
import dayjs from '../lib/dayjs'
import { UpsertBroadcastDrawer } from '../components/broadcasts/UpsertBroadcastDrawer'
import { SendOrScheduleModal } from '../components/broadcasts/SendOrScheduleModal'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { BroadcastStats, ProgressStats } from '../components/broadcasts/BroadcastStats'
import { BroadcastLinkStats } from '../components/broadcasts/BroadcastLinkStats'
import { SendingProgress } from '../components/broadcasts/SendingProgress'
import { Integration, List, Sender } from '../services/api/types'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import { Workspace, UserPermissions } from '../services/api/types'
import { Template } from '../services/api/template'
import { Template as BroadcastTemplate } from '../services/api/broadcast'
import Subtitle from '../components/common/subtitle'

const { Title, Paragraph, Text } = Typography

// Helper to convert broadcast Template to template Template
const toTemplateApiType = (template: BroadcastTemplate): Template => {
  return template as unknown as Template
}

// Helper function to calculate remaining test time
const getRemainingTestTime = (broadcast: Broadcast, testResults?: TestResultsResponse) => {
  if (
    broadcast.status !== 'testing' ||
    !broadcast.test_settings.enabled ||
    !broadcast.test_settings.test_duration_hours
  ) {
    return null
  }

  // Use test_started_at from testResults if available, otherwise use test_sent_at from broadcast
  const testStartTime = testResults?.test_started_at || broadcast.test_sent_at
  if (!testStartTime || typeof testStartTime !== 'string') {
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

// Maps a variation's aggregate stats into the shape the Variations table's Opens/Clicks
// columns (and the per-link Click rate) expect. Returns undefined for an unsent variation
// (0 sent) so those rows stay N/A instead of showing 0% / NaN.
const toVariationMetrics = (s?: MessageHistoryStatusSum) => {
  if (!s || !s.total_sent) return undefined
  return {
    recipients: s.total_sent,
    opens: s.total_opened,
    open_rate: s.total_opened / s.total_sent,
    clicks: s.total_clicked,
    click_rate: s.total_clicked / s.total_sent
  }
}

// Helper function to get status badge with tooltips
const getStatusBadge = (
  broadcast: Broadcast,
  remainingTime?: string | null,
  progressStats?: ProgressStats,
  t?: (strings: TemplateStringsArray, ...values: unknown[]) => string
) => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const translate = t || ((s: TemplateStringsArray) => s[0] as any)
  switch (broadcast.status) {
    case 'draft':
      return (
        <Tooltip title={translate`This broadcast is a draft and has not been scheduled or sent yet.`}>
          <span>
            <Badge status="default" text={translate`Draft`} />
          </span>
        </Tooltip>
      )
    case 'scheduled':
      return (
        <Tooltip title={translate`This broadcast is scheduled and will start sending at the specified time.`}>
          <span>
            <Badge status="processing" text={translate`Scheduled`} />
          </span>
        </Tooltip>
      )
    case 'processing':
      return (
        <Tooltip title={translate`Preparing emails for delivery. Contacts are being added to the sending queue.`}>
          <span>
            <Badge status="processing" text={translate`Preparing...`} />
          </span>
        </Tooltip>
      )
    case 'paused':
      return (
        <Tooltip
          title={broadcast.pause_reason || translate`Sending has been paused. You can resume at any time.`}
        >
          <Space size="small">
            <Badge status="warning" text={translate`Paused`} />
            {broadcast.pause_reason && (
              <FontAwesomeIcon
                icon={faCircleQuestion}
                className="text-orange-500 cursor-help"
                style={{ opacity: 0.7 }}
              />
            )}
          </Space>
        </Tooltip>
      )
    case 'processed': {
      if (progressStats && progressStats.remaining > 0) {
        const tooltipText = translate`Emails are being delivered. ${progressStats.processed.toLocaleString()} sent, ${progressStats.remaining.toLocaleString()} remaining.`
        return (
          <Tooltip title={tooltipText}>
            <span>
              <Badge
                status="warning"
                text={translate`Sending ${progressStats.remaining.toLocaleString()} remaining`}
              />
            </span>
          </Tooltip>
        )
      }
      const completeTooltip = progressStats
        ? translate`All ${progressStats.enqueuedCount.toLocaleString()} emails have been processed.`
        : translate`All emails have been sent.`
      return (
        <Tooltip title={completeTooltip}>
          <span>
            <Badge status="success" text={translate`Complete`} />
          </span>
        </Tooltip>
      )
    }
    case 'cancelled':
      return (
        <Tooltip title={translate`This broadcast was cancelled before completion.`}>
          <span>
            <Badge status="error" text={translate`Cancelled`} />
          </span>
        </Tooltip>
      )
    case 'failed':
      return (
        <Tooltip title={translate`This broadcast failed due to an error. Check the logs for details.`}>
          <span>
            <Badge status="error" text={translate`Failed`} />
          </span>
        </Tooltip>
      )
    case 'testing':
      return (
        <Tooltip title={translate`A/B test is in progress. Emails are being sent to the test group.`}>
          <Space size="small">
            <Badge status="processing" text={translate`A/B Testing`} />
            {remainingTime && (
              <Text type="secondary" style={{ fontSize: '12px' }}>
                ({remainingTime})
              </Text>
            )}
          </Space>
        </Tooltip>
      )
    case 'test_completed':
      return (
        <Tooltip title={translate`A/B test has completed. Select a winner to send to the remaining recipients.`}>
          <span>
            <Badge status="success" text={translate`Test Completed`} />
          </span>
        </Tooltip>
      )
    case 'winner_selected':
      return (
        <Tooltip title={translate`A winner has been selected. Emails are being sent to the remaining recipients.`}>
          <span>
            <Badge status="success" text={translate`Winner Selected`} />
          </span>
        </Tooltip>
      )
    default:
      return <Badge status="default" text={broadcast.status} />
  }
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
  currentWorkspace: Workspace | undefined
  permissions: UserPermissions | null
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
  const { t } = useLingui()
  const [showDetails, setShowDetails] = useState(isFirst)
  const queryClient = useQueryClient()
  const { message } = App.useApp()
  const [testModalOpen, setTestModalOpen] = useState(false)
  const [templateToTest, setTemplateToTest] = useState<Template | null>(null)
  const [progressStats, setProgressStats] = useState<ProgressStats | undefined>()

  const variations = broadcast.test_settings.variations || []

  // Per-variation aggregate stats. The A/B testResults endpoint only serves testing-phase
  // broadcasts, so once a broadcast is processed these power the variation Opens/Clicks
  // columns and the per-link Click rate denominator. Fetched lazily when details are open.
  const variationStatsQueries = useQueries({
    queries: variations.map((v) => ({
      queryKey: ['broadcast-variation-stats', workspaceId, broadcast.id, v.template_id],
      queryFn: () => getBroadcastVariationStats(workspaceId, broadcast.id, v.template_id),
      enabled: showDetails && broadcast.status !== 'draft' && !!v.template_id,
      staleTime: 60_000
    }))
  })
  const variationStatsById: Record<string, MessageHistoryStatusSum> = {}
  variations.forEach((v, i) => {
    const stats = variationStatsQueries[i]?.data?.stats
    if (v.template_id && stats) variationStatsById[v.template_id] = stats
  })

  // Fetch task associated with this broadcast
  const { data: task, isLoading: isTaskLoading } = useQuery({
    queryKey: ['task', workspaceId, broadcast.id],
    queryFn: () => {
      return taskApi.findByBroadcastId(workspaceId, broadcast.id)
    },
    // Only fetch task data if the broadcast status indicates a task might exist
    // enabled: ['scheduled', 'processing', 'paused', 'failed'].includes(broadcast.status),
    refetchInterval:
      broadcast.status === 'processing'
        ? 5000 // Refetch every 5 seconds for processing broadcasts
        : broadcast.status === 'scheduled'
          ? 30000 // Refetch every 30 seconds for scheduled broadcasts
          : broadcast.status === 'processed'
            ? 10000 // Mid-drain (Phase 2): poll every 10s while remaining > 0
            : false // Don't auto-refetch for terminal statuses
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

  // Get enqueued count from task state
  const enqueuedCount = task?.state?.send_broadcast?.enqueued_count

  // Handler for selecting winner
  const handleSelectWinner = async (templateId: string) => {
    try {
      await broadcastApi.selectWinner({
        workspace_id: workspaceId,
        id: broadcast.id,
        template_id: templateId
      })
      message.success(
        t`Winner selected successfully! The broadcast will be sent to remaining recipients.`
      )
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
      queryClient.invalidateQueries({ queryKey: ['testResults', workspaceId, broadcast.id] })
    } catch (error) {
      message.error(t`Failed to select winner`)
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
        return <Badge status="processing" text={t`Pending`} />
      case 'running':
        return <Badge status="processing" text={t`Running`} />
      case 'completed':
        return <Badge status="success" text={t`Completed`} />
      case 'failed':
        return <Badge status="error" text={t`Failed`} />
      case 'cancelled':
        return <Badge status="warning" text={t`Cancelled`} />
      case 'paused':
        return <Badge status="warning" text={t`Paused`} />
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
          <div className="font-medium text-gray-500">{t`Status`}</div>
          <div>{getTaskStatusBadge(task.status)}</div>
        </div>

        {task.next_run_after && task.status !== 'completed' && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">{t`Next Run`}</div>
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
            <div className="font-medium text-gray-500">{t`Progress`}</div>
            <Progress
              percent={Math.round(
                task.state?.send_broadcast
                  ? (task.state.send_broadcast.enqueued_count /
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
            <div className="font-medium text-gray-500">{t`Message`}</div>
            <div>{task.state.message}</div>
          </div>
        )}

        {task.state?.send_broadcast && task.state.send_broadcast.failed_count > 0 && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">{t`Failed`}</div>
            <div className="text-sm text-red-500">{task.state.send_broadcast.failed_count}</div>
          </div>
        )}

        {task.error_message && (
          <div className="mb-2">
            <div className="font-medium text-gray-500">{t`Error`}</div>
            <div className="text-red-500 text-sm">{task.error_message}</div>
          </div>
        )}

        {task.type && <div className="text-xs text-gray-500 mt-2">{t`Task type:`} {task.type}</div>}
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
                title={t`Task Status`}
                placement="bottom"
                trigger="hover"
              >
                <span className="cursor-help">
                  {getStatusBadge(broadcast, remainingTestTime, progressStats, t)}
                  <FontAwesomeIcon
                    icon={faCircleQuestion}
                    style={{ opacity: 0.7 }}
                    className="ml-2"
                  />
                </span>
              </Popover>
            ) : isTaskLoading ? (
              <span className="text-gray-400">
                {getStatusBadge(broadcast, remainingTestTime, progressStats, t)}
                <FontAwesomeIcon icon={faSpinner} spin className="ml-2" />
              </span>
            ) : (
              getStatusBadge(broadcast, remainingTestTime, progressStats, t)
            )}
          </div>
        </Space>
      }
      extra={
        <Space>
          <Tooltip title={t`Refresh Broadcast`}>
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
                  ? t`You don't have write permission for broadcasts`
                  : t`Edit Broadcast`
              }
            >
              <div>
                {currentWorkspace && (
                  <UpsertBroadcastDrawer
                    workspace={currentWorkspace}
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
                )}
              </div>
            </Tooltip>
          )}
          {(broadcast.status === 'processing' ||
            (broadcast.status === 'processed' && (progressStats?.remaining ?? 0) > 0)) && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? t`You don't have write permission for broadcasts`
                  : t`Pause Broadcast`
              }
            >
              <Popconfirm
                title={t`Pause broadcast?`}
                description={t`The broadcast will stop sending and can be resumed later.`}
                onConfirm={() => onPause(broadcast)}
                okText={t`Yes, pause`}
                cancelText={t`Cancel`}
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
                  ? t`You don't have write permission for broadcasts`
                  : t`Resume Broadcast`
              }
            >
              <Popconfirm
                title={t`Resume broadcast?`}
                description={t`The broadcast will continue sending from where it was paused.`}
                onConfirm={() => onResume(broadcast)}
                okText={t`Yes, resume`}
                cancelText={t`Cancel`}
                disabled={!permissions?.broadcasts?.write}
              >
                <Button type="text" size="small" disabled={!permissions?.broadcasts?.write}>
                  <FontAwesomeIcon icon={faCirclePlay} style={{ opacity: 0.7 }} />
                </Button>
              </Popconfirm>
            </Tooltip>
          )}
          {(broadcast.status === 'scheduled' ||
            broadcast.status === 'paused' ||
            broadcast.status === 'processing' ||
            (broadcast.status === 'processed' && (progressStats?.remaining ?? 0) > 0)) && (
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? t`You don't have write permission for broadcasts`
                  : t`Cancel Broadcast`
              }
            >
              <Popconfirm
                title={t`Cancel broadcast?`}
                description={t`Queued emails will not be sent. In-flight sends will complete.`}
                onConfirm={() => onCancel(broadcast)}
                okText={t`Yes, cancel`}
                cancelText={t`No`}
                disabled={!permissions?.broadcasts?.write}
              >
                <Button type="text" size="small" disabled={!permissions?.broadcasts?.write}>
                  <FontAwesomeIcon icon={faBan} style={{ opacity: 0.7 }} />
                </Button>
              </Popconfirm>
            </Tooltip>
          )}
          {broadcast.status === 'draft' && (
            <>
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? t`You don't have write permission for broadcasts`
                    : t`Delete Broadcast`
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
                    ? t`You don't have write permission for broadcasts`
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
                  {t`Send or Schedule`}
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
        {/* Show progress bar when sending */}
        {broadcast.status === 'processed' &&
          enqueuedCount &&
          progressStats &&
          progressStats.remaining > 0 && (
            <SendingProgress
              enqueuedCount={enqueuedCount}
              sentCount={progressStats.sentCount}
              failedCount={progressStats.failedCount}
              startedAt={broadcast.started_at}
            />
          )}
        <BroadcastStats
          workspaceId={workspaceId}
          broadcastId={broadcast.id}
          workspace={currentWorkspace}
          enqueuedCount={enqueuedCount}
          broadcastStatus={broadcast.status}
          onStatsUpdate={setProgressStats}
        />
      </div>

      <div className={`bg-gradient-to-br from-gray-50 to-violet-50 border-t border-gray-200`}>
        <div className="text-center py-2">
          <Button type="link" onClick={() => setShowDetails(!showDetails)}>
            {showDetails ? (
              <Space size="small">
                <FontAwesomeIcon icon={faChevronUp} style={{ opacity: 0.7 }} className="mr-1" />{' '}
                {t`Hide Details`}
              </Space>
            ) : (
              <Space size="small">
                <FontAwesomeIcon icon={faChevronDown} style={{ opacity: 0.7 }} className="mr-1" />{' '}
                {t`Show Details`}
              </Space>
            )}
          </Button>
        </div>

        {showDetails && (
          <div className="p-6">
            <div className="flex items-center gap-2 mb-2">
              <Subtitle className="!mb-0">{t`Variations`}</Subtitle>
            </div>
            <div className="mb-6">
              <Table
                showHeader={false}
                dataSource={(broadcast.test_settings.variations || []).map((variation, index) => {
                  const emailProvider = currentWorkspace?.integrations?.find(
                    (i: Integration) =>
                      i.id ===
                      (variation.template?.category === 'marketing'
                        ? currentWorkspace.settings?.marketing_email_provider_id
                        : currentWorkspace.settings?.transactional_email_provider_id)
                  )?.email_provider

                  const templateSender = emailProvider?.senders.find(
                    (s: Sender) => s.id === variation.template?.email?.sender_id
                  )

                  const variationResult = testResults?.variation_results?.[variation.template_id]
                  // Winner comes from live A/B results while testing, else from the
                  // winning_template recorded on the broadcast (top-level column) — so a
                  // sent broadcast still highlights it.
                  const isWinner =
                    (testResults?.winning_template || broadcast.winning_template) ===
                    variation.template_id

                  return {
                    key: variation.template_id || index,
                    index: index + 1,
                    isWinner,
                    templateName: variation.template?.name || t`Untitled`,
                    template: variation.template,
                    sender: templateSender
                      ? `${templateSender.name} <${templateSender.email}>`
                      : t`Default sender`,
                    subject: variation.template?.email?.subject || t`N/A`,
                    subjectPreview: variation.template?.email?.subject_preview,
                    replyTo: variation.template?.email?.reply_to || '-',
                    metrics:
                      variationResult ||
                      toVariationMetrics(variationStatsById[variation.template_id]) ||
                      variation.metrics,
                    variation,
                    templateId: variation.template_id
                  }
                })}
                columns={[
                  {
                    title: '#',
                    dataIndex: 'index',
                    key: 'index',
                    render: (index, record) => (
                      <Space>
                        {'#' + index}
                        {record.isWinner && <Badge status="success" />}
                      </Space>
                    )
                  },
                  {
                    title: t`Template`,
                    key: 'templateName',
                    render: (record) => <Tooltip title={t`Template`}>{record.templateName}</Tooltip>
                  },
                  {
                    title: t`Subject`,
                    dataIndex: 'subject',
                    key: 'subject',
                    render: (subject, record) => (
                      <div>
                        <div>{subject}</div>
                        {record.subjectPreview && (
                          <div className="text-xs text-gray-500">{record.subjectPreview}</div>
                        )}
                      </div>
                    )
                  },
                  {
                    title: t`Info`,
                    key: 'sender',
                    render: (record) => (
                      <div>
                        <div>
                          <span className="font-medium text-gray-500">{t`From:`}</span> {record.sender}
                        </div>
                        {record.replyTo && record.replyTo !== '-' && (
                          <div>
                            <span className="font-medium text-gray-500">{t`Reply To:`}</span>{' '}
                            {record.replyTo}
                          </div>
                        )}
                      </div>
                    )
                  },
                  ...(broadcast.status !== 'draft'
                    ? [
                        {
                          title: t`Opens`,
                          key: 'opens',
                          width: 80,
                          render: (_: unknown, record: { metrics?: { open_rate?: number; opens?: number; recipients?: number } }) => {
                            const metrics = record.metrics
                            if (!metrics) {
                              return (
                                <Tooltip title={t`0 opens out of 0 recipients`}>
                                  <>
                                    <FontAwesomeIcon icon={faEye} style={{ opacity: 0.7 }} className="text-purple-500" />
                                    <span className="cursor-help ml-1">{t`N/A`}</span>
                                  </>
                                </Tooltip>
                              )
                            }
                            const openRate = metrics.open_rate || 0
                            const opens = metrics.opens || 0
                            const recipients = metrics.recipients || 0
                            return (
                              <Tooltip title={t`${opens} opens out of ${recipients} recipients`}>
                                <>
                                  <FontAwesomeIcon icon={faEye} style={{ opacity: 0.7 }} className="text-purple-500" />{' '}
                                  <span className="cursor-help ml-1">
                                    {(openRate * 100).toFixed(1)}%
                                  </span>
                                </>
                              </Tooltip>
                            )
                          }
                        },
                        {
                          title: t`Clicks`,
                          key: 'clicks',
                          width: 80,
                          render: (_: unknown, record: { metrics?: { click_rate?: number; clicks?: number; recipients?: number } }) => {
                            const metrics = record.metrics
                            if (!metrics) {
                              return (
                                <Tooltip title={t`0 clicks out of 0 recipients`}>
                                  <>
                                    <FontAwesomeIcon
                                      icon={faArrowPointer}
                                      style={{ opacity: 0.7 }}
                                      className="text-cyan-500"
                                    />{' '}
                                    <span className="cursor-help ml-1">{t`N/A`}</span>
                                  </>
                                </Tooltip>
                              )
                            }
                            const clickRate = metrics.click_rate || 0
                            const clicks = metrics.clicks || 0
                            const recipients = metrics.recipients || 0
                            return (
                              <Tooltip title={t`${clicks} clicks out of ${recipients} recipients`}>
                                <>
                                  <FontAwesomeIcon icon={faArrowPointer} style={{ opacity: 0.7 }} className="text-cyan-500" />{' '}
                                  <span className="cursor-help ml-1">
                                    {(clickRate * 100).toFixed(1)}%
                                  </span>
                                </>
                              </Tooltip>
                            )
                          }
                        }
                      ]
                    : []),
                  {
                    title: t`Actions`,
                    key: 'actions',
                    width: 150,
                    align: 'right',
                    render: (_, record) => {
                      const canSelectWinner =
                        broadcast.status === 'test_completed' &&
                        !broadcast.test_settings.auto_send_winner
                      return (
                        // Stop clicks here from toggling the row's expand-on-click.
                        <Space size="small" onClick={(e) => e.stopPropagation()}>
                          {record.template && (
                            <Tooltip
                              title={
                                !(permissions?.templates?.read && permissions?.contacts?.write)
                                  ? t`You need read template and write contact permissions to send test emails`
                                  : t`Send Test Email`
                              }
                            >
                              <Button
                                size="small"
                                type="text"
                                icon={
                                  <FontAwesomeIcon icon={faPaperPlane} className="opacity-70" />
                                }
                                onClick={() => record.template && handleTestTemplate(toTemplateApiType(record.template))}
                                disabled={
                                  !(permissions?.templates?.read && permissions?.contacts?.write)
                                }
                              />
                            </Tooltip>
                          )}
                          <Tooltip title={t`Preview Template`}>
                            <>
                              {record.template && currentWorkspace ? (
                                <TemplatePreviewDrawer
                                  record={toTemplateApiType(record.template)}
                                  workspace={currentWorkspace}
                                >
                                  <Button
                                    size="small"
                                    type="text"
                                    ghost
                                    icon={<FontAwesomeIcon icon={faEye} className="opacity-70" />}
                                  />
                                </TemplatePreviewDrawer>
                              ) : (
                                <Button
                                  size="small"
                                  type="text"
                                  ghost
                                  icon={<FontAwesomeIcon icon={faEye} className="opacity-70" />}
                                  disabled
                                />
                              )}
                            </>
                          </Tooltip>
                          {canSelectWinner && record.templateId && (
                            <Tooltip
                              title={
                                !permissions?.broadcasts?.write
                                  ? t`You don't have write permission for broadcasts`
                                  : undefined
                              }
                            >
                              <Popconfirm
                                title={t`Select Winner`}
                                description={t`Are you sure you want to select "${record.templateName}" as the winner? The broadcast will be sent to the remaining recipients.`}
                                onConfirm={() => handleSelectWinner(record.templateId)}
                                okText={t`Yes, Select Winner`}
                                cancelText={t`Cancel`}
                              >
                                <Button
                                  size="small"
                                  type="primary"
                                  disabled={!permissions?.broadcasts?.write}
                                >
                                  {t`Select`}
                                </Button>
                              </Popconfirm>
                            </Tooltip>
                          )}
                        </Space>
                      )
                    }
                  }
                ]}
                size="small"
                pagination={false}
                scroll={{ x: 'max-content' }}
                rowClassName={(record) => (record.isWinner ? 'bg-green-50' : '')}
                expandable={{
                  // Only sent broadcasts have click data; a single-variation broadcast
                  // opens its row by default so the links are visible without a click.
                  rowExpandable: () => broadcast.status !== 'draft',
                  // Clicking anywhere on the row toggles it (the Actions cell stops
                  // propagation so its buttons still work).
                  expandRowByClick: true,
                  defaultExpandedRowKeys:
                    variations.length === 1 ? [variations[0].template_id || 0] : [],
                  expandedRowRender: (record) => (
                    <BroadcastLinkStats
                      workspaceId={workspaceId}
                      broadcastId={broadcast.id}
                      templateId={record.templateId}
                      recipients={record.metrics?.recipients}
                    />
                  )
                }}
              />
            </div>

            <Row gutter={32}>
              <Col span={8}>
                <Subtitle className="mt-8 mb-4">{t`Audience`}</Subtitle>

                <Descriptions bordered={false} size="small" column={1}>
                  {/* Audience Information */}
                  {broadcast.audience.segments && broadcast.audience.segments.length > 0 && (
                    <Descriptions.Item label={t`Segments`}>
                      <Space wrap>
                        {broadcast.audience.segments.map((segmentId) => {
                          const segment = segments.find((s) => s.id === segmentId)
                          return segment ? (
                            <Tag key={segment.id} color={segment.color} bordered={false}>
                              {segment.name}
                            </Tag>
                          ) : (
                            <Tag key={segmentId} bordered={false}>
                              {t`Unknown segment`} ({segmentId})
                            </Tag>
                          )
                        })}
                      </Space>
                    </Descriptions.Item>
                  )}

                  {broadcast.audience.list && (
                    <Descriptions.Item label={t`List`}>
                      {(() => {
                        const list = lists.find((l) => l.id === broadcast.audience.list)
                        return list ? list.name : t`Unknown list` + ` (${broadcast.audience.list})`
                      })()}
                    </Descriptions.Item>
                  )}

                  <Descriptions.Item label={t`Exclude Unsubscribed`}>
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
                </Descriptions>

                {/* Schedule Information */}
                {broadcast.schedule.is_scheduled &&
                  broadcast.schedule.scheduled_date &&
                  broadcast.schedule.scheduled_time && (
                    <Descriptions.Item label={t`Scheduled`}>
                      {dayjs(
                        `${broadcast.schedule.scheduled_date} ${broadcast.schedule.scheduled_time}`
                      ).format('lll')}
                      {' '}
                      {broadcast.schedule.use_recipient_timezone
                        ? t`in recipients timezone`
                        : t`in ${broadcast.schedule.timezone}`}
                    </Descriptions.Item>
                  )}

                {/* sending */}
                <Descriptions bordered={false} size="small" column={1}>
                  {broadcast.started_at && (
                    <Descriptions.Item label={t`Started`}>
                      {dayjs(broadcast.started_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {broadcast.completed_at && (
                    <Descriptions.Item label={t`Completed`}>
                      {dayjs(broadcast.completed_at).fromNow()}
                    </Descriptions.Item>
                  )}

                  {broadcast.paused_at && (
                    <Descriptions.Item label={t`Paused`}>
                      <Space direction="vertical" size="small">
                        <div>{dayjs(broadcast.paused_at).fromNow()}</div>
                        {broadcast.pause_reason && (
                          <div className="text-orange-600 text-sm">
                            <strong>{t`Reason:`}</strong> {broadcast.pause_reason}
                          </div>
                        )}
                      </Space>
                    </Descriptions.Item>
                  )}

                  {broadcast.cancelled_at && (
                    <Descriptions.Item label={t`Cancelled`}>
                      {dayjs(broadcast.cancelled_at).fromNow()}
                    </Descriptions.Item>
                  )}
                </Descriptions>
              </Col>
              <Col span={8}>
                <Subtitle className="mt-8 mb-4">{t`Web Analytics`}</Subtitle>
                <Descriptions bordered={false} size="small" column={1}>
                  {broadcast.utm_parameters?.source && (
                    <Descriptions.Item label={t`UTM Source`}>
                      {broadcast.utm_parameters.source}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.medium && (
                    <Descriptions.Item label={t`UTM Medium`}>
                      {broadcast.utm_parameters.medium}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.campaign && (
                    <Descriptions.Item label={t`UTM Campaign`}>
                      {broadcast.utm_parameters.campaign}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.term && (
                    <Descriptions.Item label={t`UTM Term`}>
                      {broadcast.utm_parameters.term}
                    </Descriptions.Item>
                  )}

                  {broadcast.utm_parameters?.content && (
                    <Descriptions.Item label={t`UTM Content`}>
                      {broadcast.utm_parameters.content}
                    </Descriptions.Item>
                  )}
                </Descriptions>
              </Col>
              <Col span={8}>
                {broadcast.test_settings.enabled && (
                  <>
                    <Subtitle className="mt-8 mb-4">{t`A/B Test`}</Subtitle>
                    <Descriptions bordered={false} size="small" column={1}>
                      <Descriptions.Item label={t`Sample Percentage`}>
                        {broadcast.test_settings.sample_percentage}%
                      </Descriptions.Item>

                      {broadcast.test_settings.auto_send_winner &&
                        broadcast.test_settings.auto_send_winner_metric &&
                        broadcast.test_settings.test_duration_hours && (
                          <Descriptions.Item label={t`Auto-send Winner`}>
                            <div className="flex items-center">
                              <FontAwesomeIcon
                                icon={faCircleCheck}
                                className="text-green-500 mr-2"
                                size="sm"
                                style={{ opacity: 0.7 }}
                              />
                              <span>
                                {t`After ${broadcast.test_settings.test_duration_hours} hours based on highest ${broadcast.test_settings.auto_send_winner_metric === 'open_rate' ? t`opens` : t`clicks`}`}
                              </span>
                            </div>
                          </Descriptions.Item>
                        )}

                      {testResults && testResults.recommended_winner && (
                        <Descriptions.Item label={t`Recommended Winner`}>
                          <Space>
                            <Badge status="processing" text={t`Recommended`} />
                            {Object.values(testResults.variation_results).find(
                              (result) => result.template_id === testResults.recommended_winner
                            )?.template_name || t`Unknown`}
                          </Space>
                        </Descriptions.Item>
                      )}

                      {testResults && testResults.winning_template && (
                        <Descriptions.Item label={t`Selected Winner`}>
                          <Space>
                            <Badge status="success" text={t`Winner`} />
                            {Object.values(testResults.variation_results).find(
                              (result) => result.template_id === testResults.winning_template
                            )?.template_name || t`Unknown`}
                          </Space>
                        </Descriptions.Item>
                      )}
                    </Descriptions>
                  </>
                )}
              </Col>
            </Row>
          </div>
        )}
      </div>

      {/* Test Template Modal */}
      {currentWorkspace && (
        <SendTemplateModal
          isOpen={testModalOpen}
          onClose={() => setTestModalOpen(false)}
          template={templateToTest}
          workspace={currentWorkspace}
        />
      )}
    </Card>
  )
}

// URL search params for the broadcasts list (status group + name search query)
interface BroadcastsSearch {
  status?: string
  q?: string
}

// Valid Segmented status group values. Anything else falls back to 'all'.
const STATUS_GROUP_VALUES = ['all', 'draft', 'scheduled', 'sending', 'sent', 'failed']

export function BroadcastsPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/broadcasts' })
  const navigate = useNavigate({ from: '/console/workspace/$workspaceId/broadcasts' })
  const routeSearch = useSearch({
    from: '/console/workspace/$workspaceId/broadcasts'
  }) as BroadcastsSearch

  // Active status group + search term derived from the URL
  const statusGroup =
    routeSearch.status && STATUS_GROUP_VALUES.includes(routeSearch.status)
      ? routeSearch.status
      : 'all'
  const searchTerm = routeSearch.q ?? ''
  const hasActiveFilter = statusGroup !== 'all' || searchTerm !== ''

  // Local mirror of the search box so typing stays responsive; the URL (and
  // therefore the query) is only updated after a short debounce.
  const [searchInput, setSearchInput] = useState(searchTerm)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  // The last value we ourselves pushed to the URL via the debounce. Lets the
  // URL->input sync effect ignore the echo of our own writes and only react to
  // external changes (back/forward, deep links), so in-progress typing is never
  // overwritten.
  const lastNavigatedQRef = useRef<string | undefined>(undefined)
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
    queryKey: ['broadcasts', workspaceId, currentPage, pageSize, statusGroup, searchTerm],
    queryFn: () => {
      return broadcastApi.list({
        workspace_id: workspaceId,
        with_templates: true,
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
        statuses: getStatusesForGroup(statusGroup),
        search: searchTerm || undefined
      })
    },
    // Keep the previous page's results visible while a new filter/search/page
    // fetches, so the filter bar and list don't flash empty on every change.
    placeholderData: keepPreviousData
  })

  // Mirror the URL query into the search box ONLY for external changes
  // (browser back/forward, deep links). Our own debounced writes are skipped
  // via lastNavigatedQRef so they never clobber what the user is typing. On a
  // genuine external change we also cancel any pending debounce (so a stale
  // timer can't navigate back over where the user just went) and re-seed the
  // ref so a later forward-nav to the same value still syncs the box.
  useEffect(() => {
    if (searchTerm !== lastNavigatedQRef.current) {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      setSearchInput(searchTerm)
      lastNavigatedQRef.current = searchTerm
    }
  }, [searchTerm])

  // Strip an unknown status value from the URL (e.g. a hand-edited or stale
  // ?status=garbage) so it doesn't linger in shareable links while the UI
  // silently treats it as "All". replace: avoids adding a history entry.
  useEffect(() => {
    if (routeSearch.status && !STATUS_GROUP_VALUES.includes(routeSearch.status)) {
      navigate({ replace: true, search: (prev) => ({ ...prev, status: undefined }) })
    }
  }, [routeSearch.status, navigate])

  // Reset to the first page whenever the active filter or search changes. This
  // also covers browser back/forward, which change the URL (and thus
  // statusGroup/searchTerm) without going through the handlers below.
  useEffect(() => {
    setCurrentPage(1)
  }, [statusGroup, searchTerm])

  // Safety net: if we land on an out-of-range page (current page is empty but
  // matches exist, e.g. after data changes), jump back to the first page so
  // results are never hidden behind a stale offset.
  useEffect(() => {
    if (
      !isLoading &&
      data &&
      (data.broadcasts?.length ?? 0) === 0 &&
      data.total_count > 0 &&
      currentPage > 1
    ) {
      setCurrentPage(1)
    }
  }, [isLoading, data, currentPage])

  // Clear any pending debounce on unmount
  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [])

  const handleStatusGroupChange = (value: string) => {
    setCurrentPage(1)
    navigate({ search: (prev) => ({ ...prev, status: value === 'all' ? undefined : value }) })
  }

  const handleSearchChange = (value: string) => {
    setSearchInput(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const trimmed = value.trim()
      // Record the value searchTerm will become so the sync effect treats this
      // as our own write and leaves the (possibly untrimmed) input untouched.
      lastNavigatedQRef.current = trimmed
      setCurrentPage(1)
      // replace: each keystroke updates the URL in place instead of pushing a
      // history entry, so Back doesn't step through every intermediate term.
      navigate({ replace: true, search: (prev) => ({ ...prev, q: trimmed || undefined }) })
    }, 300)
  }

  const handleClearFilters = () => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    lastNavigatedQRef.current = ''
    setSearchInput('')
    setCurrentPage(1)
    navigate({ search: (prev) => ({ ...prev, status: undefined, q: undefined }) })
  }

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

      message.success(t`Broadcast "${broadcastToDelete.name}" deleted successfully`)
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
      message.error(t`Failed to delete broadcast`)
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
      message.success(t`Broadcast "${broadcast.name}" paused successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error(t`Failed to pause broadcast`)
      console.error(error)
    }
  }

  const handleResumeBroadcast = async (broadcast: Broadcast) => {
    try {
      await broadcastApi.resume({
        workspace_id: workspaceId,
        id: broadcast.id
      })
      message.success(t`Broadcast "${broadcast.name}" resumed successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error(t`Failed to resume broadcast`)
      console.error(error)
    }
  }

  const handleCancelBroadcast = async (broadcast: Broadcast) => {
    try {
      await broadcastApi.cancel({
        workspace_id: workspaceId,
        id: broadcast.id
      })
      message.success(t`Broadcast "${broadcast.name}" cancelled successfully`)
      queryClient.invalidateQueries({
        queryKey: ['broadcasts', workspaceId, currentPage, pageSize]
      })
    } catch (error) {
      message.error(t`Failed to cancel broadcast`)
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
    message.success(t`Broadcast "${broadcast.name}" refreshed`)
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
        <div className="text-2xl font-medium">{t`Broadcasts`}</div>
        {currentWorkspace && (hasBroadcasts || hasActiveFilter) && (
          <Space>
            <Tooltip
              title={
                !permissions?.broadcasts?.write
                  ? t`You don't have write permission for broadcasts`
                  : undefined
              }
            >
              <div>
                <UpsertBroadcastDrawer
                  workspace={currentWorkspace}
                  lists={lists}
                  segments={segments}
                  buttonContent={<>{t`Create Broadcast`}</>}
                  buttonProps={{
                    disabled: !permissions?.broadcasts?.write
                  }}
                />
              </div>
            </Tooltip>
          </Space>
        )}
      </div>

      {(hasBroadcasts || hasActiveFilter) && (
        <div className="flex justify-between items-center mb-6 gap-4">
          <Segmented
            value={statusGroup}
            onChange={(value) => handleStatusGroupChange(value as string)}
            options={[
              { label: t`All`, value: 'all' },
              { label: t`Draft`, value: 'draft' },
              { label: t`Scheduled`, value: 'scheduled' },
              { label: t`Sending`, value: 'sending' },
              { label: t`Sent`, value: 'sent' },
              { label: t`Failed`, value: 'failed' }
            ]}
          />
          <Input.Search
            allowClear
            placeholder={t`Search by name`}
            value={searchInput}
            onChange={(e) => handleSearchChange(e.target.value)}
            onBlur={() => {
              // Reconcile the box to the trimmed term that is actually applied
              // so stray leading/trailing spaces don't linger after editing.
              const trimmed = searchInput.trim()
              if (trimmed !== searchInput) setSearchInput(trimmed)
            }}
            className="max-w-xs"
          />
        </div>
      )}

      {!hasMarketingEmailProvider && (
        <Alert
          message={t`Email Provider Required`}
          description={t`You don't have a marketing email provider configured. Please set up an email provider in your workspace settings to send broadcasts.`}
          type="warning"
          showIcon
          className="!mb-6"
          action={
            <Button
              type="primary"
              size="small"
              href={`/console/workspace/${workspaceId}/settings/integrations`}
            >
              {t`Configure Provider`}
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
                showTotal={(total, range) => t`${range[0]}-${range[1]} of ${total} broadcasts`}
              />
            </div>
          )}
        </div>
      ) : hasActiveFilter ? (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            {t`No broadcasts match your filters`}
          </Title>
          <Paragraph type="secondary">
            {t`Try adjusting your search or status filter`}
          </Paragraph>
          <div className="mt-4">
            <Button onClick={handleClearFilters}>{t`Clear filters`}</Button>
          </div>
        </div>
      ) : (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            {t`No broadcasts found`}
          </Title>
          <Paragraph type="secondary">{t`Create your first broadcast to get started`}</Paragraph>
          <div className="mt-4">
            {currentWorkspace && (
              <Tooltip
                title={
                  !permissions?.broadcasts?.write
                    ? t`You don't have write permission for broadcasts`
                    : undefined
                }
              >
                <div>
                  <UpsertBroadcastDrawer
                    workspace={currentWorkspace}
                    lists={lists}
                    segments={segments}
                    buttonContent={t`Create Broadcast`}
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
        title={t`Delete Broadcast`}
        open={deleteModalVisible}
        onCancel={closeDeleteModal}
        footer={[
          <Button key="cancel" onClick={closeDeleteModal}>
            {t`Cancel`}
          </Button>,
          <Button
            key="delete"
            type="primary"
            danger
            loading={isDeleting}
            disabled={confirmationInput !== (broadcastToDelete?.id || '')}
            onClick={handleDeleteBroadcast}
          >
            {t`Delete`}
          </Button>
        ]}
      >
        {broadcastToDelete && (
          <>
            <p>{t`Are you sure you want to delete the broadcast "${broadcastToDelete.name}"?`}</p>
            <p>
              {t`This action cannot be undone. To confirm, please enter the broadcast ID:`}{' '}
              <Text code>{broadcastToDelete.id}</Text>
              <Tooltip title={t`Copy to clipboard`}>
                <Button
                  type="text"
                  icon={<FontAwesomeIcon icon={faCopy} style={{ opacity: 0.7 }} />}
                  size="small"
                  onClick={() => {
                    navigator.clipboard.writeText(broadcastToDelete.id)
                    message.success(t`Broadcast ID copied to clipboard`)
                  }}
                />
              </Tooltip>
            </p>
            <Input
              placeholder={t`Enter broadcast ID to confirm`}
              value={confirmationInput}
              onChange={(e) => setConfirmationInput(e.target.value)}
              status={
                confirmationInput && confirmationInput !== broadcastToDelete.id ? 'error' : ''
              }
            />
            {confirmationInput && confirmationInput !== broadcastToDelete.id && (
              <p className="text-red-500 mt-2">{t`ID doesn't match`}</p>
            )}
          </>
        )}
      </Modal>
    </div>
  )
}
