import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { ABTestNodeConfig } from '../../../services/api/automation'

type ABTestNodeProps = NodeProps<AutomationFlowNode>

export const ABTestNode: React.FC<ABTestNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Structural<ABTestNodeConfig>
  const variants = config?.variants || []

  // Calculate handle positions to spread evenly across bottom
  const getHandlePosition = (index: number, total: number): number => {
    if (total === 1) return 50
    // Spread handles from 20% to 80% of width
    const start = 20
    const end = 80
    return start + (index * (end - start)) / (total - 1)
  }

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="ab_test"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {variants.length === 0 ? (
          <div className="text-warning-fg">{t`Configure`}</div>
        ) : (
          <div className="flex flex-wrap gap-2 mt-1">
            {variants.map((variant) => (
              <div
                key={variant.id}
                className="text-xs bg-fill px-2 py-1 rounded"
              >
                {variant.name}: {variant.weight}%
              </div>
            ))}
          </div>
        )}
      </BaseNode>
      {/* Multiple source handles - one per variant */}
      {variants.map((variant, index) => (
        <SourceHandle key={variant.id} id={variant.id} left={`${getHandlePosition(index, variants.length)}%`} isOrphan={data.isOrphan} />
      ))}
      {/* Default handle if no variants configured */}
      {variants.length === 0 && (
        <SourceHandle isOrphan={data.isOrphan} />
      )}
    </>
  )
}
