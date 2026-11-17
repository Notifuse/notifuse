import React, { useEffect } from 'react'
import { useUpdateNodeInternals, type NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { OutputNames } from './OutputNames'
import { outputLeft } from './outputLayout'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { ABTestNodeConfig } from '../../../services/api/automation'

type ABTestNodeProps = NodeProps<AutomationFlowNode>

export const ABTestNode: React.FC<ABTestNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Structural<ABTestNodeConfig>
  const variants = config?.variants || []
  const handleKey = variants.map((variant) => variant.id).join('|')

  // ReactFlow measures a node's handles when the node mounts or resizes, not when a handle is
  // added: a variant added from the panel keeps the node's size, so its handle stays unknown and
  // every render of its "+" link warns, many times a second. Re-measure when the handles change.
  const updateNodeInternals = useUpdateNodeInternals()
  useEffect(() => {
    updateNodeInternals(id)
  }, [id, handleKey, updateNodeInternals])

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="ab_test"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
        outputs={<OutputNames nodeType="ab_test" config={data.config} />}
      >
        {variants.length === 0 ? <div className="text-warning-fg">{t`Configure`}</div> : null}
      </BaseNode>
      {/* Multiple source handles - one per variant */}
      {variants.map((variant, index) => (
        <SourceHandle
          key={variant.id}
          id={variant.id}
          left={`${outputLeft('ab_test', index, variants.length)}%`}
          isOrphan={data.isOrphan}
        />
      ))}
      {/* Default handle if no variants configured */}
      {variants.length === 0 && (
        <SourceHandle isOrphan={data.isOrphan} />
      )}
    </>
  )
}
