import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { UserPlus } from 'lucide-react'
import { Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import { useAutomation } from '../context'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { AddToListNodeConfig } from '../../../services/api/automation'

type AddToListNodeProps = NodeProps<AutomationNodeData>

export const AddToListNode: React.FC<AddToListNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const config = data.config as AddToListNodeConfig
  const listName = lists.find((l) => l.id === config?.list_id)?.name
  const status = config?.status || 'subscribed'

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
        type="add_to_list"
        label={t`Add to List`}
        icon={
          <UserPlus
            size={16}
            style={{ color: selected ? undefined : nodeTypeColors.add_to_list }}
          />
        }
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!config?.list_id ? (
          <div className="text-orange-500">{t`Configure`}</div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="text-sm truncate max-w-[180px]">{listName || t`Unknown list`}</span>
            <Tag color={status === 'subscribed' ? 'green' : 'orange'} className="m-0">
              {status}
            </Tag>
          </div>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: sourceHandleColor, width: 10, height: 10 }}
      />
    </>
  )
}
