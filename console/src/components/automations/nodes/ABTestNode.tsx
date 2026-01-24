import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFlask } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { ABTestNodeConfig } from '../../../services/api/automation'

type ABTestNodeProps = NodeProps<AutomationNodeData>

export const ABTestNode: React.FC<ABTestNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const config = data.config as ABTestNodeConfig
  const variants = config?.variants || []

  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'
  const sourceHandleColor = data.isOrphan ? '#f97316' : '#3b82f6'

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
        type="ab_test"
        label={t`A/B Test`}
        icon={
          <FontAwesomeIcon
            icon={faFlask}
            style={{ color: selected ? undefined : nodeTypeColors.ab_test }}
          />
        }
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {variants.length === 0 ? (
          <div className="text-orange-500">{t`Configure`}</div>
        ) : (
          <div className="flex flex-wrap gap-2 mt-1">
            {variants.map((variant) => (
              <div
                key={variant.id}
                className="text-xs bg-gray-100 px-2 py-1 rounded"
              >
                {variant.name}: {variant.weight}%
              </div>
            ))}
          </div>
        )}
      </BaseNode>
      {/* Multiple source handles - one per variant */}
      {variants.map((variant, index) => (
        <Handle
          key={variant.id}
          type="source"
          position={Position.Bottom}
          id={variant.id}
          style={{
            background: sourceHandleColor,
            width: 10,
            height: 10,
            left: `${getHandlePosition(index, variants.length)}%`
          }}
        />
      ))}
      {/* Default handle if no variants configured */}
      {variants.length === 0 && (
        <Handle
          type="source"
          position={Position.Bottom}
          style={{ background: sourceHandleColor, width: 10, height: 10 }}
        />
      )}
    </>
  )
}
