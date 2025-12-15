import React from 'react'
import { Typography, Empty } from 'antd'
import { X } from 'lucide-react'
import type { Node } from '@xyflow/react'
import { TriggerConfigForm, DelayConfigForm, EmailConfigForm } from './config'
import type { AutomationNodeData } from './utils/flowConverter'
import type { DelayNodeConfig, EmailNodeConfig } from '../../services/api/automation'

const { Title } = Typography

interface NodeConfigPanelProps {
  selectedNode: Node<AutomationNodeData> | null
  onNodeUpdate: (nodeId: string, data: Partial<AutomationNodeData>) => void
  workspaceId: string
  onClose?: () => void
}

export const NodeConfigPanel: React.FC<NodeConfigPanelProps> = ({
  selectedNode,
  onNodeUpdate,
  workspaceId,
  onClose
}) => {
  if (!selectedNode) {
    return null
  }

  const { nodeType, config } = selectedNode.data

  const handleConfigChange = (newConfig: Record<string, unknown>) => {
    onNodeUpdate(selectedNode.id, {
      ...selectedNode.data,
      config: newConfig
    })
  }

  const renderConfigForm = () => {
    switch (nodeType) {
      case 'trigger':
        return (
          <TriggerConfigForm
            config={config as { event_kinds?: string[]; frequency?: 'once' | 'every_time' }}
            onChange={handleConfigChange}
          />
        )
      case 'delay':
        return (
          <DelayConfigForm
            config={config as DelayNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'email':
        return (
          <EmailConfigForm
            config={config as EmailNodeConfig}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
          />
        )
      default:
        return (
          <Empty
            description={`Configuration for ${nodeType} is not available in Phase 2`}
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        )
    }
  }

  return (
    <div className="bg-white overflow-y-auto max-h-[400px]">
      <div className="p-3 border-b border-gray-200 flex items-center justify-between">
        <Title level={5} style={{ margin: 0, fontSize: '14px' }}>
          Configure {selectedNode.data.label}
        </Title>
        {onClose && (
          <button
            onClick={onClose}
            className="p-1 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700"
          >
            <X size={16} />
          </button>
        )}
      </div>
      <div className="p-3">{renderConfigForm()}</div>
    </div>
  )
}
