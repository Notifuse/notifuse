import React, { useState } from 'react'
import {
  Card,
  Space,
  Badge,
  Button,
  Tooltip,
  Popconfirm,
  Descriptions,
  Tag,
  Statistic,
  Row,
  Col,
  Drawer
} from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCirclePause, faTrashCan, faPenToSquare } from '@fortawesome/free-regular-svg-icons'
import { PieChart } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import { AutomationFlowViewer } from './AutomationFlowViewer'
import { automationApi } from '../../services/api/automation'
import type { Automation, AutomationStatus, AutomationNodeStats } from '../../services/api/automation'
import type { UserPermissions } from '../../services/api/workspace'
import type { List } from '../../services/api/list'
import type { Segment } from '../../services/api/segment'

// Status badge is created inside component for i18n support

interface AutomationCardProps {
  automation: Automation
  lists: List[]
  segments?: Segment[]
  permissions: UserPermissions | null
  workspaceId: string
  onActivate: (automation: Automation) => void
  onPause: (automation: Automation) => void
  onDelete: (automation: Automation) => void
  onEdit: (automation: Automation) => void
}

export const AutomationCard: React.FC<AutomationCardProps> = ({
  automation,
  lists,
  segments = [],
  permissions,
  workspaceId,
  onActivate,
  onPause,
  onDelete,
  onEdit
}) => {
  const { t } = useLingui()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [nodeStats, setNodeStats] = useState<Record<string, AutomationNodeStats> | null>(null)
  const [statsLoading, setStatsLoading] = useState(false)
  const [flowHeight, setFlowHeight] = useState(300)

  // Helper function to get status badge
  const getStatusBadge = (status: AutomationStatus) => {
    switch (status) {
      case 'draft':
        return <Badge status="default" text={t`Draft`} />
      case 'live':
        return <Badge status="processing" text={t`Live`} />
      case 'paused':
        return <Badge status="warning" text={t`Paused`} />
      default:
        return <Badge status="default" text={status} />
    }
  }

  const fetchNodeStats = async () => {
    setStatsLoading(true)
    try {
      const response = await automationApi.getNodeStats({
        workspace_id: workspaceId,
        automation_id: automation.id
      })
      setNodeStats(response.node_stats)
    } catch (error) {
      console.error('Failed to fetch node stats:', error)
      setNodeStats({}) // Set empty to prevent re-fetching
    } finally {
      setStatsLoading(false)
    }
  }

  const handleOpenDrawer = () => {
    setDrawerOpen(true)
    // Always fetch fresh stats when opening drawer
    setNodeStats(null)
    fetchNodeStats()
  }

  // Find the list name if list_id is set
  const listName = automation.list_id
    ? lists.find((l) => l.id === automation.list_id)?.name || automation.list_id
    : t`No list`

  // Get trigger event kind and filter info
  const triggerEvent = automation.trigger?.event_kind
  const triggerListId = automation.trigger?.list_id
  const triggerSegmentId = automation.trigger?.segment_id
  const triggerCustomEventName = automation.trigger?.custom_event_name

  // Build trigger filter display
  const getTriggerFilterDisplay = () => {
    if (!triggerEvent) return null

    if (triggerEvent.startsWith('list.') && triggerListId) {
      const listItem = lists.find((l) => l.id === triggerListId)
      return listItem?.name || triggerListId
    }
    if (triggerEvent.startsWith('segment.') && triggerSegmentId) {
      const segmentItem = segments.find((s) => s.id === triggerSegmentId)
      return segmentItem?.name || triggerSegmentId
    }
    if (triggerEvent === 'custom_event' && triggerCustomEventName) {
      return triggerCustomEventName
    }
    return null
  }

  const triggerFilter = getTriggerFilterDisplay()

  return (
    <Card
      styles={{
        body: {
          padding: 0
        }
      }}
      title={
        <Space size="large">
          <div>{automation.name}</div>
          <div className="text-xs font-normal">{getStatusBadge(automation.status)}</div>
        </Space>
      }
      extra={
        <Space>
          {/* Delete button - for draft and paused */}
          {(automation.status === 'draft' || automation.status === 'paused') && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? t`You don't have write permission for automations`
                  : t`Delete Automation`
              }
            >
              <Popconfirm
                title={t`Delete automation?`}
                description={t`This action cannot be undone.`}
                onConfirm={() => onDelete(automation)}
                okText={t`Yes, delete`}
                okButtonProps={{ danger: true }}
                cancelText={t`Cancel`}
                disabled={!permissions?.automations?.write}
              >
                <Button
                  type="text"
                  size="small"
                  icon={<FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />}
                  disabled={!permissions?.automations?.write}
                />
              </Popconfirm>
            </Tooltip>
          )}

          {/* Edit button - only for draft/paused */}
          {(automation.status === 'draft' || automation.status === 'paused') && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? t`You don't have write permission for automations`
                  : t`Edit Automation`
              }
            >
              <Button
                type="text"
                size="small"
                icon={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                onClick={() => onEdit(automation)}
                disabled={!permissions?.automations?.write}
              />
            </Tooltip>
          )}

          {/* Stats button - always visible */}
          <Tooltip title={t`View Flow Stats`}>
            <Button
              type="text"
              size="small"
              icon={<PieChart size={14} style={{ opacity: 0.7 }} />}
              onClick={handleOpenDrawer}
            />
          </Tooltip>

          {/* Activate button - for draft and paused */}
          {(automation.status === 'draft' || automation.status === 'paused') && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? t`You don't have write permission for automations`
                  : t`Activate Automation`
              }
            >
              <Popconfirm
                title={t`Activate automation?`}
                description={t`The automation will start processing contacts that match the trigger.`}
                onConfirm={() => onActivate(automation)}
                okText={t`Yes, activate`}
                cancelText={t`Cancel`}
                disabled={!permissions?.automations?.write}
              >
                <Button type="primary" size="small" disabled={!permissions?.automations?.write}>
                  {t`Activate`}
                </Button>
              </Popconfirm>
            </Tooltip>
          )}

          {/* Pause button - only for live */}
          {automation.status === 'live' && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? t`You don't have write permission for automations`
                  : t`Pause Automation`
              }
            >
              <Popconfirm
                title={t`Pause automation?`}
                description={t`The automation will stop processing new contacts.`}
                onConfirm={() => onPause(automation)}
                okText={t`Yes, pause`}
                cancelText={t`Cancel`}
                disabled={!permissions?.automations?.write}
              >
                <Button
                  type="text"
                  size="small"
                  icon={<FontAwesomeIcon icon={faCirclePause} style={{ opacity: 0.7 }} />}
                  disabled={!permissions?.automations?.write}
                />
              </Popconfirm>
            </Tooltip>
          )}
        </Space>
      }
      key={automation.id}
      className="!mb-6"
    >
      {/* Stats Row */}
      {automation.stats && (
        <div className="px-6 py-4 border-b border-gray-100">
          <Row gutter={24}>
            <Col span={6}>
              <Statistic
                title={t`Enrolled`}
                value={automation.stats.enrolled}
                valueStyle={{ fontSize: '20px' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title={t`Completed`}
                value={automation.stats.completed}
                valueStyle={{ fontSize: '20px', color: '#52c41a' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title={t`Exited`}
                value={automation.stats.exited}
                valueStyle={{ fontSize: '20px', color: '#faad14' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title={t`Failed`}
                value={automation.stats.failed}
                valueStyle={{ fontSize: '20px', color: '#ff4d4f' }}
              />
            </Col>
          </Row>
        </div>
      )}

      {/* Details */}
      <div className="px-6 py-4 border-b border-gray-100">
        <Descriptions size="small" column={2}>
          <Descriptions.Item label={t`ID`}>{automation.id}</Descriptions.Item>
          <Descriptions.Item label={t`Trigger`}>
            <Space size="small">
              {triggerEvent ? (
                <>
                  <Tag color="blue">{triggerEvent}</Tag>
                  {triggerFilter && <Tag color="cyan">{triggerFilter}</Tag>}
                </>
              ) : (
                <span className="text-gray-400">{t`No trigger configured`}</span>
              )}
            </Space>
          </Descriptions.Item>
          <Descriptions.Item label={t`List`}>{listName}</Descriptions.Item>
          <Descriptions.Item label={t`Frequency`}>
            {automation.trigger?.frequency === 'once' ? t`Once per contact` : t`Every time`}
          </Descriptions.Item>
          <Descriptions.Item label={t`Updated`}>{dayjs(automation.updated_at).fromNow()}</Descriptions.Item>
        </Descriptions>
      </div>

      {/* Flow Stats Drawer */}
      <Drawer
        title={t`Flow Stats: ${automation.name}`}
        placement="right"
        width="100%"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      >
        <div style={{ height: 'calc(100vh - 120px)', overflowY: 'auto' }}>
          <div style={{ height: flowHeight }}>
            <AutomationFlowViewer
              automation={automation}
              nodeStats={nodeStats}
              loading={statsLoading}
              onHeightCalculated={setFlowHeight}
            />
          </div>
        </div>
      </Drawer>
    </Card>
  )
}
