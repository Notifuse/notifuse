import React from 'react'
import { Typography, Empty } from 'antd'
import { X } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { Node } from '@xyflow/react'
import {
  TriggerConfigForm,
  DelayConfigForm,
  EmailConfigForm,
  ABTestConfigForm,
  AddToListConfigForm,
  RemoveFromListConfigForm,
  FilterConfigForm,
  WebhookConfigForm,
  ListStatusBranchConfigForm
} from './config'
import { useAutomation } from './context'
import type { AutomationNodeData } from './utils/flowConverter'
import type {
  DelayNodeConfig,
  EmailNodeConfig,
  ABTestNodeConfig,
  AddToListNodeConfig,
  RemoveFromListNodeConfig,
  FilterNodeConfig,
  WebhookNodeConfig,
  ListStatusBranchNodeConfig
} from '../../services/api/automation'

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
  const { t } = useLingui()
  const { workspace } = useAutomation()

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
            config={config as { event_kind?: string; list_id?: string; segment_id?: string; custom_event_name?: string; updated_fields?: string[]; frequency?: 'once' | 'every_time' }}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
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
            workspace={workspace}
          />
        )
      case 'ab_test':
        return (
          <ABTestConfigForm
            config={config as ABTestNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'add_to_list':
        return (
          <AddToListConfigForm
            config={config as AddToListNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'remove_from_list':
        return (
          <RemoveFromListConfigForm
            config={config as RemoveFromListNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'filter':
        return (
          <FilterConfigForm
            config={config as FilterNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'webhook':
        return (
          <WebhookConfigForm
            config={config as WebhookNodeConfig}
            onChange={handleConfigChange}
          />
        )
      case 'list_status_branch':
        return (
          <ListStatusBranchConfigForm
            config={config as ListStatusBranchNodeConfig}
            onChange={handleConfigChange}
          />
        )
      default:
        return (
          <Empty
            description={t`Configuration for ${nodeType} is not available in Phase 2`}
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        )
    }
  }

  return (
    <div className="bg-white h-full flex flex-col">
      <div className="p-3 border-b border-gray-200 flex items-center justify-between flex-shrink-0">
        <Title level={5} style={{ margin: 0, fontSize: '14px' }}>
          {t`Configure`} {selectedNode.data.label}
        </Title>
        {onClose && (
          <button
            onClick={onClose}
            className="p-1 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700 cursor-pointer"
          >
            <X size={16} />
          </button>
        )}
      </div>
      <div className="p-3 overflow-y-auto flex-1">{renderConfigForm()}</div>
    </div>
  )
}
