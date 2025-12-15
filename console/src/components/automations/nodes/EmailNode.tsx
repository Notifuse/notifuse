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

  return (
    <>
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: nodeTypeColors.email, width: 6, height: 6 }}
      />
      <BaseNode
        type="email"
        label="Email"
        icon={<Mail size={16} color={selected ? undefined : nodeTypeColors.email} />}
        selected={selected}
      >
        {hasTemplate ? (
          <div className="truncate max-w-[80px]">Template set</div>
        ) : (
          <div className="text-orange-500">Select</div>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: nodeTypeColors.email, width: 6, height: 6 }}
      />
    </>
  )
}
