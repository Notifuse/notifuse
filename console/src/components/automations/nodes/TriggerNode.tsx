import React from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Zap } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { BaseNode } from './BaseNode'
import { nodeTypeColors } from './constants'
import { useAutomation } from '../context'
import type { AutomationNodeData } from '../utils/flowConverter'

type TriggerNodeProps = NodeProps<AutomationNodeData>

interface TriggerConfig {
  event_kind?: string
  frequency?: string
  list_id?: string
  segment_id?: string
  custom_event_name?: string
}

export const TriggerNode: React.FC<TriggerNodeProps> = ({ data, selected }) => {
  const { t } = useLingui()
  const { lists, segments } = useAutomation()
  const config = data.config as TriggerConfig
  const hasEventKind = !!config.event_kind
  const frequency = config.frequency === 'every_time' ? t`Every time` : t`Once`

  // Format event kind for display (e.g., "list.subscribed" -> "List Subscribed")
  const formatEventKind = (eventKind: string): string => {
    if (eventKind === 'custom_event') return t`Custom Event`
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
        type="trigger"
        label={t`Trigger`}
        icon={<Zap size={16} color={selected ? undefined : nodeTypeColors.trigger} />}
        selected={selected}
      >
        {hasEventKind ? (
          <div>
            <div>{getEventDisplay()}</div>
            {detail && <div className="text-gray-500">{detail}</div>}
            <div className="text-gray-400">{frequency}</div>
          </div>
        ) : (
          <div className="text-orange-500">{t`Configure`}</div>
        )}
      </BaseNode>
      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: '#3b82f6', width: 10, height: 10 }}
      />
    </>
  )
}
