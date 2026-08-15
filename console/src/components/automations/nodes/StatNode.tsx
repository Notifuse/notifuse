import React from 'react'
import { Handle, Position, type Node, type NodeProps } from '@xyflow/react'
import { Statistic } from 'antd'
import {
  Play,
  Clock,
  Mail,
  GitBranch,
  Filter,
  ListPlus,
  ListMinus,
  ListChecks,
  FlaskConical,
  Webhook
} from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { nodeTypeColors, getNodeDescription } from './constants'
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
  webhook: <Webhook size={16} />,
  list_status_branch: <ListChecks size={16} />
}

// Labels are generated inside component for i18n support

// Type alias rather than an interface: ReactFlow constrains node data to `Record<string, unknown>`,
// which only an object-literal type alias satisfies (an interface has no implicit index signature).
export type StatNodeData = {
  nodeType: NodeType
  label?: string
  stats?: AutomationNodeStats
  config?: Record<string, unknown>
}

// The read-only stat canvas' node type. In ReactFlow v12 the props helper takes the node type, not
// the data type, so components below are typed `NodeProps<StatFlowNode>`.
export type StatFlowNode = Node<StatNodeData>

type StatNodeProps = NodeProps<StatFlowNode>

// Shared by all four stat node shapes below, which otherwise repeat this header verbatim. min-w-0
// is what lets the truncation take effect inside the flex row.
const StatNodeHeader: React.FC<{
  color: string
  icon: React.ReactNode
  label: string
  description?: string
}> = ({ color, icon, label, description }) => (
  <div className="flex items-center gap-2 px-3 py-2" style={{ borderBottom: `1px solid ${color}20` }}>
    <span style={{ color }}>{icon}</span>
    <div className="min-w-0">
      <div className="text-sm font-medium text-gray-800 truncate">{label}</div>
      {description && (
        <div className="text-xs text-gray-500 truncate" title={description}>
          {description}
        </div>
      )}
    </div>
  </div>
)

export const StatNode: React.FC<StatNodeProps> = ({ data }) => {
  const { t } = useLingui()
  const { nodeType, label, stats, config } = data
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
    webhook: t`Webhook`,
    list_status_branch: t`List Status`
  }

  const nodeLabel = label || nodeLabels[nodeType]

  // Use 0 values when no stats available
  const nodeStats = stats || { entered: 0, completed: 0, failed: 0, skipped: 0 }

  // Only email and webhook nodes surface a Failed counter. Any node kind can record a failed
  // execution, so a failure on another kind is counted by the API but not shown here.
  const showFailedRate = nodeType === 'email' || nodeType === 'webhook'

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
        <StatNodeHeader
          color={color}
          icon={icon}
          label={nodeLabel}
          description={getNodeDescription(config)}
        />

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              styles={{ content: { fontSize: 14, color: '#374151' } }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              styles={{ content: { fontSize: 14, color: '#16a34a' } }}
            />
            {showFailedRate && (
              <Statistic
                title={t`Failed`}
                value={nodeStats.failed}
                styles={{ content: { fontSize: 14, color: '#dc2626' } }}
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
  const { stats, config } = data
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
        <StatNodeHeader
          color={color}
          icon={<Filter size={16} />}
          label={t`Filter`}
          description={getNodeDescription(config)}
        />

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              styles={{ content: { fontSize: 14, color: '#374151' } }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              styles={{ content: { fontSize: 14, color: '#16a34a' } }}
            />
            <Statistic
              title={t`Failed`}
              value={nodeStats.skipped}
              styles={{ content: { fontSize: 14, color: '#ea580c' } }}
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

// For list status branch nodes that have three outputs
export const ListStatusStatNode: React.FC<StatNodeProps> = ({ data }) => {
  const { t } = useLingui()
  const { stats, config } = data
  const color = nodeTypeColors.list_status_branch

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
        <StatNodeHeader
          color={color}
          icon={<ListChecks size={16} />}
          label={t`List Status`}
          description={getNodeDescription(config)}
        />

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              styles={{ content: { fontSize: 14, color: '#374151' } }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              styles={{ content: { fontSize: 14, color: '#16a34a' } }}
            />
          </div>
        </div>

        {/* Branch labels */}
        <div className="px-3 py-1.5 border-t border-gray-100 flex justify-between text-xs">
          <span className="text-gray-500">{t`Not in List`}</span>
          <span className="text-green-600">{t`Active`}</span>
          <span className="text-orange-500">{t`Non-Active`}</span>
        </div>
      </div>
      {/* Three branch handles — the offsets below are mirrored by layoutNodes,
          which positions each child from the handle's X, so they must stay in sync */}
      <Handle
        type="source"
        position={Position.Bottom}
        id="not_in_list"
        style={{ background: '#9ca3af', width: 8, height: 8, left: '20%' }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="active"
        style={{ background: '#22c55e', width: 8, height: 8, left: '50%' }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="non_active"
        style={{ background: '#f97316', width: 8, height: 8, left: '80%' }}
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
        <StatNodeHeader
          color={color}
          icon={<FlaskConical size={16} />}
          label={t`A/B Test`}
          description={getNodeDescription(config)}
        />

        {/* Stats */}
        <div className="px-3 py-2 bg-gray-50">
          <div className="flex items-center justify-between">
            <Statistic
              title={t`Inflight`}
              value={nodeStats.entered}
              styles={{ content: { fontSize: 14, color: '#374151' } }}
            />
            <Statistic
              title={t`Completed`}
              value={nodeStats.completed}
              styles={{ content: { fontSize: 14, color: '#16a34a' } }}
            />
            {nodeStats.failed > 0 && (
              <Statistic
                title={t`Failed`}
                value={nodeStats.failed}
                styles={{ content: { fontSize: 14, color: '#dc2626' } }}
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
