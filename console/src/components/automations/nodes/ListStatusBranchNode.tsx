import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { ListChecks } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import { useAutomation } from '../context'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { ListStatusBranchNodeConfig } from '../../../services/api/automation'

type ListStatusBranchNodeProps = NodeProps<AutomationNodeData>

export const ListStatusBranchNode: React.FC<ListStatusBranchNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const config = data.config as ListStatusBranchNodeConfig
  const listName = lists.find((l) => l.id === config?.list_id)?.name

  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'

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
        type="list_status_branch"
        label={t`List Status`}
        icon={
          <ListChecks
            size={14}
            style={{ color: selected ? undefined : nodeTypeColors.list_status_branch }}
          />
        }
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!config?.list_id ? (
          <div className="text-orange-500 text-xs">{t`Select a list`}</div>
        ) : (
          <div className="text-xs text-gray-600 truncate max-w-[180px]">
            {t`Check`}: {listName || config.list_id}
          </div>
        )}
        {/* Branch labels */}
        <div className="flex justify-between text-xs mt-2 px-1">
          <span className="text-gray-500 font-medium">{t`Not in List`}</span>
          <span className="text-green-600 font-medium">{t`Active`}</span>
          <span className="text-orange-500 font-medium">{t`Non-Active`}</span>
        </div>
      </BaseNode>
      {/* Three source handles */}
      <Handle
        type="source"
        position={Position.Bottom}
        id="not_in_list"
        style={{
          background: '#9ca3af', // gray for not in list
          width: 10,
          height: 10,
          left: '20%'
        }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="active"
        style={{
          background: '#22c55e', // green for active
          width: 10,
          height: 10,
          left: '50%'
        }}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="non_active"
        style={{
          background: '#f97316', // orange for non-active
          width: 10,
          height: 10,
          left: '80%'
        }}
      />
    </>
  )
}
