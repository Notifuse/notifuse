import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { Tag } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import { useAutomation } from '../context'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { AddToListNodeConfig } from '../../../services/api/automation'

type AddToListNodeProps = NodeProps<AutomationFlowNode>

export const AddToListNode: React.FC<AddToListNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const config = data.config as Structural<AddToListNodeConfig>
  const listName = lists.find((l) => l.id === config?.list_id)?.name
  const status = config?.status || 'active'

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="add_to_list"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!config?.list_id ? (
          <div className="text-warning-fg">{t`Configure`}</div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="text-sm truncate max-w-[180px]">{listName || t`Unknown list`}</span>
            <Tag color={status === 'active' ? 'green' : 'orange'} className="m-0">
              {status === 'active' ? t`Subscribed` : t`Pending`}
            </Tag>
          </div>
        )}
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
