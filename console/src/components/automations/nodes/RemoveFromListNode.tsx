import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { UserMinus } from 'lucide-react'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import { useAutomation } from '../context'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { RemoveFromListNodeConfig } from '../../../services/api/automation'

type RemoveFromListNodeProps = NodeProps<AutomationNodeData>

export const RemoveFromListNode: React.FC<RemoveFromListNodeProps> = ({ data, selected }) => {
  const { lists } = useAutomation()
  const config = data.config as RemoveFromListNodeConfig
  const listName = lists.find((l) => l.id === config?.list_id)?.name

  const handleColor = data.isOrphan ? '#f97316' : '#3b82f6'

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: handleColor, width: 10, height: 10 }}
      />
      <BaseNode
        type="remove_from_list"
        label="Remove from List"
        icon={
          <UserMinus
            size={16}
            style={{ color: selected ? undefined : nodeTypeColors.remove_from_list }}
          />
        }
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!config?.list_id ? (
          <div className="text-orange-500">Configure</div>
        ) : (
          <span className="text-sm truncate max-w-[200px]">{listName || 'Unknown list'}</span>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: handleColor, width: 10, height: 10 }}
      />
    </>
  )
}
