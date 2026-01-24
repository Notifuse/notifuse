import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faHourglass } from '@fortawesome/free-regular-svg-icons'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { DelayNodeConfig } from '../../../services/api/automation'

type DelayNodeProps = NodeProps<AutomationNodeData>

export const DelayNode: React.FC<DelayNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const config = data.config as DelayNodeConfig
  const duration = config?.duration || 0
  const unit = config?.unit || 'minutes'

  const formatDuration = () => {
    if (duration === 0) return t`Configure`
    const unitLabel = duration === 1 ? unit.slice(0, -1) : unit
    return `${duration} ${unitLabel}`
  }

  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'
  const sourceHandleColor = data.isOrphan ? '#f97316' : '#3b82f6'

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
        type="delay"
        label={t`Delay`}
        icon={<FontAwesomeIcon icon={faHourglass} style={{ color: selected ? undefined : nodeTypeColors.delay }} />}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        <div className={duration === 0 ? 'text-orange-500' : ''}>
          {formatDuration()}
        </div>
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: sourceHandleColor, width: 10, height: 10 }}
      />
    </>
  )
}
