import React from 'react'
import { Typography, Empty, Popconfirm } from 'antd'
import { X, Trash2 } from 'lucide-react'
import type { Node } from '@xyflow/react'
import { TriggerConfigForm, DelayConfigForm, EmailConfigForm } from './config'
import type { AutomationNodeData } from './utils/flowConverter'
import type { DelayNodeConfig, EmailNodeConfig } from '../../services/api/automation'

const { Title } = Typography

interface NodeConfigPanelProps {
  selectedNode: Node<AutomationNodeData> | null
  onNodeUpdate: (nodeId: string, data: Partial<AutomationNodeData>) => void
  onNodeDelete?: (nodeId: string) => void
  workspaceId: string
  onClose?: () => void
}

export const NodeConfigPanel: React.FC<NodeConfigPanelProps> = ({
  selectedNode,
  onNodeUpdate,
  onNodeDelete,
  workspaceId,
  onClose
}) => {
  if (!selectedNode) {
    return null
  }

  const { nodeType, config } = selectedNode.data
  const canDelete = nodeType !== 'trigger'

  const handleDelete = () => {
    onNodeDelete?.(selectedNode.id)
  }

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
            config={config as { event_kind?: string; list_id?: string; segment_id?: string; custom_event_name?: string; frequency?: 'once' | 'every_time' }}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
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
    <div className="bg-white h-full flex flex-col">
      <div className="p-3 border-b border-gray-200 flex items-center justify-between flex-shrink-0">
        <Title level={5} style={{ margin: 0, fontSize: '14px' }}>
          Configure {selectedNode.data.label}
        </Title>
        <div className="flex items-center gap-1">
          {canDelete && onNodeDelete && (
            <Popconfirm
              title="Delete node"
              description="Are you sure you want to delete this node?"
              onConfirm={handleDelete}
              okText="Delete"
              cancelText="Cancel"
              okButtonProps={{ danger: true }}
            >
              <button
                className="p-1 hover:bg-red-50 rounded text-gray-400 hover:text-red-500 cursor-pointer"
              >
                <Trash2 size={16} />
              </button>
            </Popconfirm>
          )}
          {onClose && (
            <button
              onClick={onClose}
              className="p-1 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700 cursor-pointer"
            >
              <X size={16} />
            </button>
          )}
        </div>
      </div>
      <div className="p-3 overflow-y-auto flex-1">{renderConfigForm()}</div>
    </div>
  )
}
