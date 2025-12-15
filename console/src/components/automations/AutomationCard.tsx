import React, { useState } from 'react'
import { Card, Space, Badge, Button, Tooltip, Popconfirm, Descriptions, Tag, Statistic, Row, Col } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCirclePause,
  faCirclePlay,
  faTrashCan,
  faPenToSquare
} from '@fortawesome/free-regular-svg-icons'
import { faChevronDown, faChevronUp } from '@fortawesome/free-solid-svg-icons'
import dayjs from '../../lib/dayjs'
import type { Automation, AutomationStatus, NodeType } from '../../services/api/automation'
import type { UserPermissions } from '../../services/api/workspace'
import type { List } from '../../services/api/list'

// Helper function to get status badge
const getStatusBadge = (status: AutomationStatus) => {
  switch (status) {
    case 'draft':
      return <Badge status="default" text="Draft" />
    case 'live':
      return <Badge status="processing" text="Live" />
    case 'paused':
      return <Badge status="warning" text="Paused" />
    default:
      return <Badge status="default" text={status} />
  }
}

// Helper to get node type display name
const getNodeTypeName = (type: NodeType): string => {
  const names: Record<NodeType, string> = {
    trigger: 'Trigger',
    delay: 'Delay',
    email: 'Email',
    branch: 'Branch',
    filter: 'Filter',
    add_to_list: 'Add to List',
    remove_from_list: 'Remove from List',
    ab_test: 'A/B Test'
  }
  return names[type] || type
}

// Helper to build workflow summary text
const getWorkflowSummary = (automation: Automation): string => {
  if (!automation.nodes || automation.nodes.length === 0) {
    return 'No nodes configured'
  }

  const nodeCount = automation.nodes.length
  const nodeTypes = automation.nodes.map((n) => getNodeTypeName(n.type))

  // Build a simple flow representation
  if (nodeTypes.length <= 5) {
    return `${nodeCount} nodes: ${nodeTypes.join(' → ')}`
  }

  return `${nodeCount} nodes: ${nodeTypes.slice(0, 4).join(' → ')} → ...`
}

interface AutomationCardProps {
  automation: Automation
  lists: List[]
  permissions: UserPermissions | null
  onActivate: (automation: Automation) => void
  onPause: (automation: Automation) => void
  onDelete: (automation: Automation) => void
  onEdit: (automation: Automation) => void
  isFirst?: boolean
}

export const AutomationCard: React.FC<AutomationCardProps> = ({
  automation,
  lists,
  permissions,
  onActivate,
  onPause,
  onDelete,
  onEdit,
  isFirst = false
}) => {
  const [showDetails, setShowDetails] = useState(isFirst)

  // Find the list name if list_id is set
  const listName = automation.list_id
    ? lists.find((l) => l.id === automation.list_id)?.name || automation.list_id
    : 'No list'

  // Get trigger event kinds
  const triggerEvents = automation.trigger?.event_kinds || []

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
          {/* Edit button - only for draft/paused */}
          {(automation.status === 'draft' || automation.status === 'paused') && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? "You don't have write permission for automations"
                  : 'Edit Automation'
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

          {/* Activate button - only for draft */}
          {automation.status === 'draft' && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? "You don't have write permission for automations"
                  : 'Activate Automation'
              }
            >
              <Popconfirm
                title="Activate automation?"
                description="The automation will start processing contacts that match the trigger."
                onConfirm={() => onActivate(automation)}
                okText="Yes, activate"
                cancelText="Cancel"
                disabled={!permissions?.automations?.write}
              >
                <Button
                  type="text"
                  size="small"
                  icon={<FontAwesomeIcon icon={faCirclePlay} style={{ opacity: 0.7 }} />}
                  disabled={!permissions?.automations?.write}
                />
              </Popconfirm>
            </Tooltip>
          )}

          {/* Pause button - only for live */}
          {automation.status === 'live' && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? "You don't have write permission for automations"
                  : 'Pause Automation'
              }
            >
              <Popconfirm
                title="Pause automation?"
                description="The automation will stop processing new contacts."
                onConfirm={() => onPause(automation)}
                okText="Yes, pause"
                cancelText="Cancel"
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

          {/* Delete button - only for draft */}
          {automation.status === 'draft' && (
            <Tooltip
              title={
                !permissions?.automations?.write
                  ? "You don't have write permission for automations"
                  : 'Delete Automation'
              }
            >
              <Popconfirm
                title="Delete automation?"
                description="This action cannot be undone."
                onConfirm={() => onDelete(automation)}
                okText="Yes, delete"
                okButtonProps={{ danger: true }}
                cancelText="Cancel"
                disabled={!permissions?.automations?.write}
              >
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />}
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
              <Statistic title="Enrolled" value={automation.stats.enrolled} valueStyle={{ fontSize: '20px' }} />
            </Col>
            <Col span={6}>
              <Statistic
                title="Completed"
                value={automation.stats.completed}
                valueStyle={{ fontSize: '20px', color: '#52c41a' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="Exited"
                value={automation.stats.exited}
                valueStyle={{ fontSize: '20px', color: '#faad14' }}
              />
            </Col>
            <Col span={6}>
              <Statistic
                title="Failed"
                value={automation.stats.failed}
                valueStyle={{ fontSize: '20px', color: '#ff4d4f' }}
              />
            </Col>
          </Row>
        </div>
      )}

      {/* Summary Info */}
      <div className="px-6 py-4 border-b border-gray-100">
        <Space direction="vertical" size="small" className="w-full">
          <div className="flex items-center gap-2">
            <span className="text-gray-500 text-sm">Trigger:</span>
            <Space size="small">
              {triggerEvents.length > 0 ? (
                triggerEvents.map((event) => (
                  <Tag key={event} color="blue">
                    {event}
                  </Tag>
                ))
              ) : (
                <span className="text-gray-400 text-sm">No trigger configured</span>
              )}
            </Space>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-gray-500 text-sm">List:</span>
            <span className="text-sm">{listName}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-gray-500 text-sm">Workflow:</span>
            <span className="text-sm text-gray-600">{getWorkflowSummary(automation)}</span>
          </div>
        </Space>
      </div>

      {/* Collapsible Details */}
      <div
        className="px-6 py-3 cursor-pointer hover:bg-gray-50 flex items-center justify-between"
        onClick={() => setShowDetails(!showDetails)}
      >
        <span className="text-sm text-gray-500">Details</span>
        <FontAwesomeIcon
          icon={showDetails ? faChevronUp : faChevronDown}
          className="text-gray-400"
          size="sm"
        />
      </div>

      {showDetails && (
        <div className="px-6 py-4 bg-gray-50">
          <Descriptions size="small" column={2}>
            <Descriptions.Item label="ID">{automation.id}</Descriptions.Item>
            <Descriptions.Item label="Status">{automation.status}</Descriptions.Item>
            <Descriptions.Item label="Trigger Frequency">
              {automation.trigger?.frequency === 'once' ? 'Once per contact' : 'Every time'}
            </Descriptions.Item>
            <Descriptions.Item label="Total Nodes">
              {automation.nodes?.length || 0}
            </Descriptions.Item>
            <Descriptions.Item label="Created">
              {dayjs(automation.created_at).format('lll')}
            </Descriptions.Item>
            <Descriptions.Item label="Updated">
              {dayjs(automation.updated_at).format('lll')}
            </Descriptions.Item>
          </Descriptions>
        </div>
      )}
    </Card>
  )
}
