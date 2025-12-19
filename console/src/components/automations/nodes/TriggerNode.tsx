import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Zap } from 'lucide-react'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'

type TriggerNodeProps = NodeProps<AutomationNodeData>

export const TriggerNode: React.FC<TriggerNodeProps> = ({ data, selected }) => {
  const config = data.config as { event_kinds?: string[]; frequency?: string }
  const eventCount = config.event_kinds?.length || 0
  const frequency = config.frequency === 'every_time' ? 'Every time' : 'Once'

  return (
    <>
      <BaseNode
        type="trigger"
        label="Trigger"
        icon={<Zap size={16} color={selected ? undefined : nodeTypeColors.trigger} />}
        selected={selected}
      >
        {eventCount > 0 ? (
          <div>
            <span>{eventCount} event{eventCount !== 1 ? 's' : ''}</span>
            <span className="text-gray-400 ml-1">· {frequency}</span>
          </div>
        ) : (
          <div className="text-orange-500">Configure</div>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: '#3b82f6', width: 10, height: 10 }}
      />
    </>
  )
}
