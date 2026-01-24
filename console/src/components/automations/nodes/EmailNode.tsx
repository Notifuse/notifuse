import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { Mail } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import { useAutomation } from '../context'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { EmailNodeConfig } from '../../../services/api/automation'

type EmailNodeProps = NodeProps<AutomationNodeData>

export const EmailNode: React.FC<EmailNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const { templates } = useAutomation()
  const config = data.config as EmailNodeConfig
  const hasTemplate = !!config?.template_id
  const templateName = config?.template_id ? templates.find(tmpl => tmpl.id === config.template_id)?.name : undefined
  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'
  const sourceHandleColor = data.isOrphan ? '#f97316' : '#3b82f6'

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
        type="email"
        label={t`Email`}
        icon={<Mail size={16} color={selected ? undefined : nodeTypeColors.email} />}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {hasTemplate ? (
          <div>{templateName || t`Template set`}</div>
        ) : (
          <div className="text-orange-500">{t`Select`}</div>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: sourceHandleColor, width: 10, height: 10 }}
      />
    </>
  )
}
