import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { OutputNames } from './OutputNames'
import { outputLeft } from './outputLayout'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { FilterNodeConfig } from '../../../services/api/automation'

type FilterNodeProps = NodeProps<AutomationFlowNode>

export const FilterNode: React.FC<FilterNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Structural<FilterNodeConfig>
  const hasConditions = config?.conditions !== undefined

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
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="filter"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
        outputs={<OutputNames nodeType="filter" config={data.config} />}
      >
        {!hasConditions ? (
          <div className="text-warning-fg text-xs">{t`No conditions`}</div>
        ) : (
          // Just the count: the description used to be folded into this line, but it is now
          // rendered by BaseNode like every other node type's.
          <div className="text-xs text-ink-2">
            {`${conditionCount} ${conditionCount !== 1 ? t`conditions` : t`condition`}`}
          </div>
        )}
      </BaseNode>
      {/* Two fixed source handles: continue (Yes) and exit (No) */}
      <SourceHandle id="continue" left={`${outputLeft('filter', 0, 2)}%`} tone="yes" />
      <SourceHandle id="exit" left={`${outputLeft('filter', 1, 2)}%`} tone="no" />
    </>
  )
}
