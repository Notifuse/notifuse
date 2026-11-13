import React from 'react'
import type { NodeProps } from '@xyflow/react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { SourceHandle } from './handles'
import { getNodeDescription } from './constants'
import { useAutomation } from '../context'
import type { AutomationFlowNode } from '../utils/flowConverter'
import type { TimelineTriggerConfig } from '../../../services/api/automation'

type TriggerNodeProps = NodeProps<AutomationFlowNode>

// The trigger's config bag is only partially filled until the trigger is configured, so read it as
// a Partial of the API's trigger config rather than assuming event_kind/frequency are present.
type TriggerConfig = Partial<TimelineTriggerConfig>

export const TriggerNode: React.FC<TriggerNodeProps> = ({ id, data, selected }) => {
  const { t } = useLingui()
  const { lists, segments } = useAutomation()
  const config = data.config as TriggerConfig
  const hasEventKind = !!config.event_kind
  const frequency = config.frequency === 'every_time' ? t`Every time` : t`Once`

  // Format event kind for display (e.g., "list.subscribed" -> "List Subscribed")
  const formatEventKind = (eventKind: string): string => {
    if (eventKind === 'custom_event') return t`Custom Event`
    if (eventKind === 'email.link_clicked') return t`Email Link Clicked`
    const parts = eventKind.split('.')
    return parts.map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
  }

  // Look up list/segment names
  const listName = config.list_id ? lists.find(l => l.id === config.list_id)?.name : undefined
  const segmentName = config.segment_id ? segments.find(s => s.id === config.segment_id)?.name : undefined

  // Build detail string (for list/segment names shown on separate line)
  const getDetailString = () => {
    if (config.event_kind?.startsWith('list.') && listName) {
      return listName
    }
    if (config.event_kind?.startsWith('segment.') && segmentName) {
      return segmentName
    }
    if (config.event_kind === 'email.link_clicked') {
      const link = config.link_url
      return link ? t`Link contains ${link}` : t`Any link`
    }
    return null
  }

  const detail = getDetailString()

  // Build event display string
  const getEventDisplay = () => {
    if (config.event_kind === 'custom_event' && config.custom_event_name) {
      return t`Custom Event: ${config.custom_event_name}`
    }
    return formatEventKind(config.event_kind!)
  }

  return (
    <>
      <BaseNode
        nodeId={id}
        type="trigger"
        description={getNodeDescription(data.config)}
        selected={selected}
      >
        {hasEventKind ? (
          <div>
            <div>{getEventDisplay()}</div>
            {detail && <div className="text-ink-3 truncate" title={detail}>{detail}</div>}
            <div className="text-ink-3">{frequency}</div>
          </div>
        ) : (
          <div className="text-warning-fg">{t`Configure`}</div>
        )}
      </BaseNode>
      <SourceHandle isOrphan={data.isOrphan} />
    </>
  )
}
