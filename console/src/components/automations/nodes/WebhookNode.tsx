import React from 'react'
import { Handle, Position, useConnection, type NodeProps } from '@xyflow/react'
import { Globe } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import type { AutomationNodeData } from '../utils/flowConverter'
import type { WebhookNodeConfig } from '../../../services/api/automation'

type WebhookNodeProps = NodeProps<AutomationNodeData>

export const WebhookNode: React.FC<WebhookNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const config = data.config as WebhookNodeConfig
  const hasUrl = !!config?.url
  const connection = useConnection()
  const isConnecting = connection.inProgress
  const targetHandleSize = isConnecting ? 16 : 10
  const targetHandleColor = isConnecting ? '#22c55e' : data.isOrphan ? '#f97316' : '#3b82f6'
  const sourceHandleColor = data.isOrphan ? '#f97316' : '#3b82f6'

  // Truncate URL for display
  const displayUrl = config?.url
    ? config.url.length > 30
      ? config.url.substring(0, 30) + '...'
      : config.url
    : undefined

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
        type="webhook"
        label={t`Webhook`}
        icon={<Globe size={16} color={selected ? undefined : nodeTypeColors.webhook} />}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {hasUrl ? (
          <div className="truncate max-w-[200px]" title={config.url}>
            {displayUrl}
          </div>
        ) : (
          <div className="text-orange-500">{t`Configure`}</div>
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
