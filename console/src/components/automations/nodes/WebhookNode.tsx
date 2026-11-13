import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { WebhookNodeConfig } from '../../../services/api/automation'
import { refusedWebhookHost } from '../utils/webhookTarget'

type WebhookNodeProps = NodeProps<AutomationFlowNode>

export const WebhookNode: React.FC<WebhookNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const config = data.config as Structural<WebhookNodeConfig>
  const hasUrl = !!config?.url
  // Shown on the canvas, not only in the form, so a node whose URL the server
  // refuses is visible as soon as the automation is opened.
  const refused = hasUrl && refusedWebhookHost(config.url) !== null

  // Truncate URL for display
  const displayUrl = config?.url
    ? config.url.length > 30
      ? config.url.substring(0, 30) + '...'
      : config.url
    : undefined

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="webhook"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {hasUrl ? (
          <>
            <div
              className={refused ? 'truncate max-w-[200px] text-danger-fg' : 'truncate max-w-[200px]'}
              title={config.url}
            >
              {displayUrl}
            </div>
            {refused && <div className="text-danger-fg">{t`Refused: private address`}</div>}
          </>
        ) : (
          <div className="text-warning-fg">{t`Configure`}</div>
        )}
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
