import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { BaseNode } from './BaseNode'
import { OutputNames } from './OutputNames'
import { outputLeft } from './outputLayout'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { BranchNodeConfig } from '../../../services/api/automation'

type BranchNodeProps = NodeProps<AutomationFlowNode>

/** A branch made through the API: one output per path, named by its id as the edges are. */
export const BranchNode: React.FC<BranchNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Partial<Structural<BranchNodeConfig>>
  const paths = config?.paths ?? []

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="branch"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
        outputs={<OutputNames nodeType="branch" config={data.config} />}
      >
        {paths.length === 0 ? (
          <div className="text-warning-fg">{t`Configure`}</div>
        ) : (
          <div>{plural(paths.length, { one: '# path', other: '# paths' })}</div>
        )}
      </BaseNode>
      {paths.map((path, index) => (
        <SourceHandle
          key={path.id}
          id={path.id}
          left={`${outputLeft('branch', index, paths.length)}%`}
          isOrphan={data.isOrphan}
        />
      ))}
      {paths.length === 0 && <SourceHandle isOrphan={data.isOrphan} />}
    </>
  )
}
