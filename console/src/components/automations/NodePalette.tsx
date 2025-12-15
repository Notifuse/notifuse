import React from 'react'
import { Typography, Tooltip } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faClock } from '@fortawesome/free-regular-svg-icons'
import { faEnvelope } from '@fortawesome/free-regular-svg-icons'
import type { NodeType } from '../../services/api/automation'
import { nodeTypeColors } from './nodes/constants'

const { Text } = Typography

interface NodePaletteItem {
  type: NodeType
  label: string
  icon: React.ReactNode
  description: string
}

// Phase 2: Core nodes only (trigger is auto-added, not in palette)
// Note: Exit is implicit - any node without a next node terminates the automation
const PALETTE_ITEMS: NodePaletteItem[] = [
  {
    type: 'delay',
    label: 'Delay',
    icon: <FontAwesomeIcon icon={faClock} style={{ color: nodeTypeColors.delay }} />,
    description: 'Wait for a specified duration'
  },
  {
    type: 'email',
    label: 'Email',
    icon: <FontAwesomeIcon icon={faEnvelope} style={{ color: nodeTypeColors.email }} />,
    description: 'Send an email to the contact'
  }
]

interface NodePaletteProps {
  onDragStart?: (event: React.DragEvent, nodeType: NodeType) => void
}

export const NodePalette: React.FC<NodePaletteProps> = ({ onDragStart }) => {
  const handleDragStart = (event: React.DragEvent, nodeType: NodeType) => {
    event.dataTransfer.setData('application/reactflow', nodeType)
    event.dataTransfer.effectAllowed = 'move'
    onDragStart?.(event, nodeType)
  }

  return (
    <div className="h-full bg-gray-50 border-r border-gray-200 overflow-y-auto">
      <div className="p-4 border-b border-gray-200">
        <Text strong>Nodes</Text>
        <div className="text-xs text-gray-500 mt-1">Drag to add to canvas</div>
      </div>
      <div className="p-3 space-y-2">
        {PALETTE_ITEMS.map((item) => (
          <Tooltip key={item.type} title={item.description} placement="right">
            <div
              className="flex items-center gap-3 p-3 bg-white border border-gray-200 rounded-md cursor-grab hover:border-blue-300 hover:shadow-sm transition-all"
              draggable
              onDragStart={(e) => handleDragStart(e, item.type)}
            >
              <span className="text-lg">{item.icon}</span>
              <span className="text-sm font-medium">{item.label}</span>
            </div>
          </Tooltip>
        ))}
      </div>
      <div className="p-3 border-t border-gray-200 mt-auto">
        <Text type="secondary" className="text-xs">
          Note: Trigger node is automatically added when creating a new automation
        </Text>
      </div>
    </div>
  )
}
