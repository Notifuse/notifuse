import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faHourglass } from '@fortawesome/free-regular-svg-icons'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { DelayNodeConfig } from '../../../services/api/automation'

type DelayNodeProps = NodeProps<AutomationNodeData>

export const DelayNode: React.FC<DelayNodeProps> = ({ data, selected }) => {
  const config = data.config as DelayNodeConfig
  const duration = config?.duration || 0
  const unit = config?.unit || 'minutes'

  const formatDuration = () => {
    if (duration === 0) return 'Configure'
    const unitLabel = duration === 1 ? unit.slice(0, -1) : unit
    return `${duration} ${unitLabel}`
  }

  const handleColor = data.isOrphan ? '#f97316' : '#3b82f6'

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: handleColor, width: 10, height: 10 }}
      />
      <BaseNode
        type="delay"
        label="Delay"
        icon={<FontAwesomeIcon icon={faHourglass} style={{ color: selected ? undefined : nodeTypeColors.delay }} />}
        selected={selected}
        isOrphan={data.isOrphan}
      >
        <div className={duration === 0 ? 'text-orange-500' : ''}>
          {formatDuration()}
        </div>
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: handleColor, width: 10, height: 10 }}
      />
    </>
  )
}
