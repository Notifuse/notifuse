import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import { useAutomation } from '../context'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { RemoveFromListNodeConfig } from '../../../services/api/automation'

type RemoveFromListNodeProps = NodeProps<AutomationFlowNode>

export const RemoveFromListNode: React.FC<RemoveFromListNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const config = data.config as Structural<RemoveFromListNodeConfig>
  const listName = lists.find((l) => l.id === config?.list_id)?.name

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="remove_from_list"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {!config?.list_id ? (
          <div className="text-warning-fg">{t`Configure`}</div>
        ) : (
          <span className="text-sm truncate max-w-[200px]">{listName || t`Unknown list`}</span>
        )}
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
