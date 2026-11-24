import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { DelayNodeConfig } from '../../../services/api/automation'

type DelayNodeProps = NodeProps<AutomationFlowNode>

export const DelayNode: React.FC<DelayNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Structural<DelayNodeConfig>
  const duration = config?.duration || 0
  const unit = config?.unit || 'minutes'

  const formatDuration = () => {
    if (duration === 0) return t`Configure`
    switch (unit) {
      case 'days':
        return plural(duration, { one: '# day', other: '# days' })
      case 'hours':
        return plural(duration, { one: '# hour', other: '# hours' })
      default:
        return plural(duration, { one: '# minute', other: '# minutes' })
    }
  }

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="delay"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        <div className={duration === 0 ? 'text-warning-fg' : ''}>
          {formatDuration()}
        </div>
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
