import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Statistic } from 'antd'
import {
  Play,
  Clock,
  Mail,
  GitBranch,
  Filter,
  ListPlus,
  ListMinus,
  FlaskConical,
  Webhook
} from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { nodeTypeColors } from './constants'
import type { NodeType, AutomationNodeStats, ABTestNodeConfig } from '../../../services/api/automation'

// Icons for each node type
const nodeIcons: Record<NodeType, React.ReactNode> = {
  trigger: <Play size={16} />,
  delay: <Clock size={16} />,
  email: <Mail size={16} />,
  branch: <GitBranch size={16} />,
  filter: <Filter size={16} />,
  add_to_list: <ListPlus size={16} />,
  remove_from_list: <ListMinus size={16} />,
  ab_test: <FlaskConical size={16} />,
  webhook: <Webhook size={16} />
}

// Labels are generated inside component for i18n support

export interface StatNodeData {
  nodeType: NodeType
  label?: string
  stats?: AutomationNodeStats
  config?: Record<string, unknown>
}

type StatNodeProps = NodeProps<StatNodeData>

export const StatNode: React.FC<StatNodeProps> = ({ data }) => {
  const { t } = useLingui()
  const { nodeType, label, stats } = data
  const color = nodeTypeColors[nodeType] || '#6b7280'
  const icon = nodeIcons[nodeType]

  // Labels for each node type
  const nodeLabels: Record<NodeType, string> = {
    trigger: t`Trigger`,
    delay: t`Delay`,
    email: t`Email`,
    branch: t`Branch`,
    filter: t`Filter`,
    add_to_list: t`Add to List`,
    remove_from_list: t`Remove from List`,
    ab_test: t`A/B Test`,
    webhook: t`Webhook`
  }

  const nodeLabel = label || nodeLabels[nodeType]

  // Use 0 values when no stats available
  const nodeStats = stats || { entered: 0, completed: 0, failed: 0, skipped: 0 }

  // Calculate percentages for email and webhook nodes
  const showFailedRate = nodeType === 'email' || nodeType === 'webhook'
  const total = nodeStats.entered
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- Reserved for future rate display
  const _completedRate = total > 0 ? Math.round((nodeStats.completed / total) * 100) : 0
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- Reserved for future rate display
  const _failedRate = total > 0 ? Math.round((nodeStats.failed / total) * 100) : 0

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: color, width: 8, height: 8 }}
      />
      <div
        className="bg-white rounded shadow-sm"
        style={{
          width: '220px',
          border: `1px solid ${color}30`,
          overflow: 'hidden'
        }}
      >
        {/* Header */}
        <div
          className="flex items-center gap-2 px-3 py-2"
          style={{ borderBottom: `1px solid ${color}20` }}
        >
          <span style={{ color }}>{icon}</span>
          <span className="text-sm font-medium text-gray-800 truncate">{nodeLabel}</span>
        </div>

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              valueStyle={{ fontSize: 14, color: '#374151' }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              valueStyle={{ fontSize: 14, color: '#16a34a' }}
            />
            {showFailedRate && (
              <Statistic
                title={t`Failed`}
                value={nodeStats.failed}
                valueStyle={{ fontSize: 14, color: '#dc2626' }}
              />
            )}
          </div>
        </div>
      </div>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: color, width: 8, height: 8 }}
      />
    </>
  )
}

// For filter nodes that have multiple outputs
export const FilterStatNode: React.FC<StatNodeProps> = ({ data }) => {
  const { t } = useLingui()
  const { stats } = data
  const color = nodeTypeColors.filter

  // Use 0 values when no stats available
  const nodeStats = stats || { entered: 0, completed: 0, failed: 0, skipped: 0 }

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: color, width: 8, height: 8 }}
      />
      <div
        className="bg-white rounded shadow-sm"
        style={{
          width: '220px',
          border: `1px solid ${color}30`,
          overflow: 'hidden'
        }}
      >
        {/* Header */}
        <div
          className="flex items-center gap-2 px-3 py-2"
          style={{ borderBottom: `1px solid ${color}20` }}
        >
          <span style={{ color }}><Filter size={16} /></span>
          <span className="text-sm font-medium text-gray-800">{t`Filter`}</span>
        </div>

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              valueStyle={{ fontSize: 14, color: '#374151' }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              valueStyle={{ fontSize: 14, color: '#16a34a' }}
            />
            <Statistic
              title={t`Failed`}
              value={nodeStats.skipped}
              valueStyle={{ fontSize: 14, color: '#ea580c' }}
            />
          </div>
        </div>
      </div>
      {/* Yes/No handles */}
      <Handle
        type="source"
        position={Position.Bottom}
        id="yes"
        style={{ background: '#22c55e', width: 8, height: 8, left: '30%' }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="no"
        style={{ background: '#ef4444', width: 8, height: 8, left: '70%' }}
      />
    </>
  )
}

// For A/B test nodes that have multiple variant outputs
export const ABTestStatNode: React.FC<StatNodeProps> = ({ data }) => {
  const { t } = useLingui()
  const { stats, config } = data
  const color = nodeTypeColors.ab_test

  // Use 0 values when no stats available
  const nodeStats = stats || { entered: 0, completed: 0, failed: 0, skipped: 0 }

  // Get variants from config
  const abConfig = config as ABTestNodeConfig | undefined
  const variants = abConfig?.variants || []

  // Calculate handle positions based on number of variants
  const getHandlePosition = (index: number, total: number) => {
    if (total === 1) return 50
    const step = 60 / (total - 1) // Spread across 60% of width (20% to 80%)
    return 20 + step * index
  }

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: color, width: 8, height: 8 }}
      />
      <div
        className="bg-white rounded shadow-sm"
        style={{
          width: '220px',
          border: `1px solid ${color}30`,
          overflow: 'hidden'
        }}
      >
        {/* Header */}
        <div
          className="flex items-center gap-2 px-3 py-2"
          style={{ borderBottom: `1px solid ${color}20` }}
        >
          <span style={{ color }}><FlaskConical size={16} /></span>
          <span className="text-sm font-medium text-gray-800">{t`A/B Test`}</span>
        </div>

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              valueStyle={{ fontSize: 14, color: '#374151' }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              valueStyle={{ fontSize: 14, color: '#16a34a' }}
            />
            {nodeStats.failed > 0 && (
              <Statistic
                title={t`Failed`}
                value={nodeStats.failed}
                valueStyle={{ fontSize: 14, color: '#dc2626' }}
              />
            )}
          </div>
        </div>

        {/* Variant labels */}
        {variants.length > 0 && (
          <div className="px-3 py-1.5 border-t border-gray-100 flex justify-between text-xs text-gray-500">
            {variants.map((v) => (
              <span key={v.id} className="flex-1 text-center truncate">{v.name}</span>
            ))}
          </div>
        )}
      </div>
      {/* Variant handles */}
      {variants.map((variant, index) => (
        <Handle
          key={variant.id}
          type="source"
          position={Position.Bottom}
          id={variant.id}
          style={{
            background: color,
            width: 8,
            height: 8,
            left: `${getHandlePosition(index, variants.length)}%`
          }}
        />
      ))}
      {/* Fallback single handle if no variants */}
      {variants.length === 0 && (
        <Handle
          type="source"
          position={Position.Bottom}
          style={{ background: color, width: 8, height: 8 }}
        />
      )}
    </>
  )
}
