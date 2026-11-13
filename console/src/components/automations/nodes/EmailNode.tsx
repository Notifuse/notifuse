import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle, TargetHandle } from './handles'
import { getNodeDescription } from './constants'
import { useAutomation } from '../context'
import type { AutomationFlowNode, Structural } from '../utils/flowConverter'
import type { EmailNodeConfig } from '../../../services/api/automation'

type EmailNodeProps = NodeProps<AutomationFlowNode>

export const EmailNode: React.FC<EmailNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const { templates, canReadTemplates } = useAutomation()
  const config = data.config as Structural<EmailNodeConfig>
  const hasTemplate = !!config?.template_id
  const templateName = config?.template_id ? templates.find(tmpl => tmpl.id === config.template_id)?.name : undefined

  return (
    <>
      <TargetHandle isOrphan={data.isOrphan} />
      <BaseNode
        nodeId={id}
        type="email"
        description={getNodeDescription(data.config)}
        selected={selected}
        isOrphan={data.isOrphan}
        onDelete={data.onDelete}
      >
        {hasTemplate ? (
          // Without Templates read the templates are not listed: the ID says which.
          <div>{canReadTemplates ? templateName || t`Template set` : config.template_id}</div>
        ) : (
          <div className="text-warning-fg">{t`Select`}</div>
        )}
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
