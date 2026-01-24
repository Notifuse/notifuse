import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { Filter } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { FilterNodeConfig } from '../../../services/api/automation'

type FilterNodeProps = NodeProps<AutomationNodeData>

export const FilterNode: React.FC<FilterNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const config = data.config as FilterNodeConfig
  const hasConditions = config?.conditions !== undefined

  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'

  // Count conditions for display
  const countConditions = (node: FilterNodeConfig['conditions']): number => {
    if (!node) return 0
    if (node.kind === 'leaf') return 1
    if (node.kind === 'branch' && node.branch?.leaves) {
      return node.branch.leaves.reduce((sum, leaf) => sum + countConditions(leaf), 0)
    }
    return 0
  }

  const conditionCount = countConditions(config?.conditions)

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{
          background: targetHandleColor,
          width: targetHandleSize,
          height: targetHandleSize,
          transition: 'all 0.15s ease'
        }}
      />
      <BaseNode
        type="filter"
        label={t`Filter`}
        icon={
          <Filter
            size={14}
            style={{ color: selected ? undefined : nodeTypeColors.filter }}
          />
        }
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!hasConditions ? (
          <div className="text-orange-500 text-xs">{t`No conditions`}</div>
        ) : (
          <div
            className="text-xs text-gray-600 truncate max-w-[180px]"
            title={
              config.description
                ? `${config.description} (${conditionCount} ${conditionCount !== 1 ? t`conditions` : t`condition`})`
                : undefined
            }
          >
            {config.description ? (
              <>
                {config.description}
                <span className="text-gray-400 ml-1">({conditionCount})</span>
              </>
            ) : (
              `${conditionCount} ${conditionCount !== 1 ? t`conditions` : t`condition`}`
            )}
          </div>
        )}
        {/* Yes/No labels for handles */}
        <div className="flex justify-between text-xs mt-2 px-4">
          <span className="text-green-600 font-medium">{t`Yes`}</span>
          <span className="text-red-500 font-medium">{t`No`}</span>
        </div>
      </BaseNode>
      {/* Two fixed source handles: continue (Yes) and exit (No) */}
      <Handle
        type="source"
        position={Position.Bottom}
        id="continue"
        style={{
          background: '#22c55e', // green for Yes
          width: 10,
          height: 10,
          left: '30%'
        }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="exit"
        style={{
          background: '#ef4444', // red for No
          width: 10,
          height: 10,
          left: '70%'
        }}
      />
    </>
  )
}
