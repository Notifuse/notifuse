import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Mail } from 'lucide-react'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { EmailNodeConfig } from '../../../services/api/automation'

type EmailNodeProps = NodeProps<AutomationNodeData>

export const EmailNode: React.FC<EmailNodeProps> = ({ data, selected }) => {
  const config = data.config as EmailNodeConfig
  const hasTemplate = !!config?.template_id
  const handleColor = data.isOrphan ? '#f97316' : '#3b82f6'

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: handleColor, width: 10, height: 10 }}
      />
      <BaseNode
        type="email"
        label="Email"
        icon={<Mail size={16} color={selected ? undefined : nodeTypeColors.email} />}
        selected={selected}
        isOrphan={data.isOrphan}
      >
        {hasTemplate ? (
          <div>Template set</div>
        ) : (
          <div className="text-orange-500">Select</div>
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
