import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { OutputNames } from './OutputNames'
import { outputLeft } from './outputLayout'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import { useAutomation } from '../context'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { ListStatusBranchNodeConfig } from '../../../services/api/automation'

type ListStatusBranchNodeProps = NodeProps<AutomationFlowNode>

export const ListStatusBranchNode: React.FC<ListStatusBranchNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const { lists } = useAutomation()
  const config = data.config as Structural<ListStatusBranchNodeConfig>
  const listName = lists.find((l) => l.id === config?.list_id)?.name

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="list_status_branch"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
        outputs={<OutputNames nodeType="list_status_branch" config={data.config} />}
      >
        {!config?.list_id ? (
          <div className="text-warning-fg text-xs">{t`Select a list`}</div>
        ) : (
          <div className="text-xs text-ink-2 truncate max-w-[180px]">
            {t`Check`}: {listName || config.list_id}
          </div>
        )}
      </BaseNode>
      {/* Three source handles */}
      <SourceHandle id="not_in_list" left={`${outputLeft('list_status_branch', 0, 3)}%`} tone="muted" />
      <SourceHandle id="active" left={`${outputLeft('list_status_branch', 1, 3)}%`} tone="yes" />
      <SourceHandle id="non_active" left={`${outputLeft('list_status_branch', 2, 3)}%`} tone="warning" />
    </>
  )
}
