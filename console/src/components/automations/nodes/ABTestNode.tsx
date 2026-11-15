import React from 'react'
import type { NodeProps } from '@xyflow/react'
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
