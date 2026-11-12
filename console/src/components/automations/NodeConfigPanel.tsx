import React from 'react'
import { Button, ConfigProvider, Form, Input, Popconfirm, Skeleton } from 'antd'
import { Trash2, X } from 'lucide-react'
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
import { NODE_TYPES } from './nodeTypes'
import { NodeTypeChip } from './AddNodeButton'
import { getNodeDescription } from './nodes/constants'
import { useNodeStatsState } from './stats/nodeStatsState'
import { StepStats } from './stats/StepStats'
import { EmailStepStats } from './stats/EmailStepStats'
import { EmailStepPreview } from './stats/EmailStepPreview'
import { isSmtpMarketingProvider } from '../broadcasts/access'
import type { AutomationNodeData, Structural } from './utils/flowConverter'
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
import type { TreeNode } from '../../services/api/segment'

// Mirrors TriggerConfigForm's own trigger config shape, which that module does not export.
type TriggerFormConfig = {
  event_kind?: string
  list_id?: string
  segment_id?: string
  custom_event_name?: string
  updated_fields?: string[]
  conditions?: TreeNode
  frequency?: 'once' | 'every_time'
}

// The per-type forms declare `onChange` against their own concrete config interface. An interface
// carries no implicit index signature, so a `(config: Record<string, unknown>) => void` handler is
// assignable to none of them. `Structural<T>` re-expresses each config as an anonymous object type
// with the same members: it accepts the interface on the way in (parameters are contravariant) and
// satisfies `Record<string, unknown>` on the way back out into the node's config bag.
type NodeConfigUpdate =
  | TriggerFormConfig
  | Structural<DelayNodeConfig>
  | Structural<EmailNodeConfig>
  | Structural<ABTestNodeConfig>
  | Structural<AddToListNodeConfig>
  | Structural<RemoveFromListNodeConfig>
  | Structural<FilterNodeConfig>
  | Structural<WebhookNodeConfig>
  | Structural<ListStatusBranchNodeConfig>

interface NodeConfigPanelProps {
  selectedNode: Node<AutomationNodeData> | null
  onNodeUpdate: (nodeId: string, data: Partial<AutomationNodeData>) => void
  workspaceId: string
  onClose?: () => void
  /** Removes the step; absent read-only. */
  onDelete?: (nodeId: string) => void
}

export const NodeConfigPanel: React.FC<NodeConfigPanelProps> = ({
  selectedNode,
  onNodeUpdate,
  workspaceId,
  onClose,
  onDelete
}) => {
  const { t, i18n } = useLingui()
  const { workspace, canReadTemplates, readOnly, automationId } = useAutomation()
  const stats = useNodeStatsState()

  if (!selectedNode) {
    return null
  }

  const { nodeType, config } = selectedNode.data

  const writeConfig = (newConfig: Record<string, unknown>) => {
    onNodeUpdate(selectedNode.id, {
      ...selectedNode.data,
      config: newConfig
    })
  }

  const handleConfigChange = (newConfig: NodeConfigUpdate) => {
    writeConfig(newConfig)
  }

  const renderConfigForm = () => {
    switch (nodeType) {
      case 'trigger':
        return (
          <TriggerConfigForm
            config={config as TriggerFormConfig}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
            canReadTemplates={canReadTemplates}
          />
        )
      case 'delay':
        return (
          <DelayConfigForm
            config={config as Structural<DelayNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'email':
        return (
          <EmailConfigForm
            config={config as Structural<EmailNodeConfig>}
            onChange={handleConfigChange}
            workspaceId={workspaceId}
            workspace={workspace}
            canReadTemplates={canReadTemplates}
          />
        )
      case 'ab_test':
        return (
          <ABTestConfigForm
            config={config as Structural<ABTestNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'add_to_list':
        return (
          <AddToListConfigForm
            config={config as Structural<AddToListNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'remove_from_list':
        return (
          <RemoveFromListConfigForm
            config={config as Structural<RemoveFromListNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'filter':
        return (
          <FilterConfigForm
            config={config as Structural<FilterNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'webhook':
        return (
          <WebhookConfigForm
            config={config as Structural<WebhookNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      case 'list_status_branch':
        return (
          <ListStatusBranchConfigForm
            config={config as Structural<ListStatusBranchNodeConfig>}
            onChange={handleConfigChange}
          />
        )
      default:
        // A branch is made through the API, and edited there.
        return <p data-unreset className="m-0 text-sm text-ink-3">{t`This step is edited through the API.`}</p>
    }
  }

  const title = getNodeDescription(config) ?? i18n._(NODE_TYPES[nodeType].label)

  // The step's results: nothing for a draft no contact entered, a word for an unsaved step.
  const resultsBlock = () => {
    if (stats.neverRan) return null
    const heading = stats.period === 'all' ? t`All time` : t`Last 30 days`
    let body: React.ReactNode
    if (!stats.savedNodeIds.has(selectedNode.id)) {
      body = <p data-unreset className="m-0 text-sm text-ink-3">{t`Save to start counting this step`}</p>
    } else if (stats.loading) {
      body = <Skeleton active title={false} paragraph={{ rows: 3 }} />
    } else if (stats.error) {
      body = <p data-unreset className="m-0 text-sm text-ink-3">{t`Couldn't load stats`}</p>
    } else {
      const nodeStats = stats.nodes?.[selectedNode.id] ?? {
        node_type: nodeType,
        visits: 0,
        completed: 0,
        failed_attempts: 0,
        skipped: 0,
        skip_reasons: {},
        errors: 0,
        failed_journeys: 0,
        in_progress: 0,
        waiting: 0,
        retrying: 0,
        branches: {}
      }
      if (nodeType === 'email') {
        const emailConfig = config as Partial<EmailNodeConfig>
        const integration = emailConfig.integration_id
          ? workspace.integrations?.find((i) => i.id === emailConfig.integration_id)
          : undefined
        const smtp = integration ? integration.email_provider?.kind === 'smtp' : isSmtpMarketingProvider(workspace)
        const params = new URLSearchParams({ automation_id: automationId })
        if (emailConfig.template_id) params.set('template_id', emailConfig.template_id)
        body = (
          <div className="flex flex-col gap-3">
            <EmailStepStats
              email={nodeStats.email}
              canReadHistory={stats.canReadHistory}
              smtp={smtp}
              messagesHref={`/console/workspace/${workspace.id}/logs?${params.toString()}`}
            />
            <StepStats nodeType={nodeType} config={config} stats={nodeStats} />
          </div>
        )
      } else {
        body = <StepStats nodeType={nodeType} config={config} stats={nodeStats} />
      }
    }
    return (
      <section className="flex flex-col gap-2 border-t border-line-soft px-4 py-3" aria-label={heading} data-step-results>
        <h3 data-unreset className="text-xs font-medium tracking-wide text-ink-3 uppercase">
          {heading}
        </h3>
        {body}
      </section>
    )
  }

  return (
    <div className="flex h-full flex-col bg-surface" data-step-panel>
      <div className="flex shrink-0 items-center gap-2 border-b border-line-soft px-4 py-3">
        <NodeTypeChip type={nodeType} />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-xs font-medium tracking-wide text-ink-3 uppercase">{i18n._(NODE_TYPES[nodeType].label)}</span>
          <span className="truncate text-sm font-medium text-ink">{title}</span>
        </div>
        {onClose && (
          <Button type="text" size="small" aria-label={t`Close`} icon={<X size={16} aria-hidden="true" />} onClick={onClose} />
        )}
      </div>
      <div className="flex-1 overflow-y-auto">
        {/* key by node id so switching between same-type nodes remounts the form
            and resets any internal form state (defense-in-depth for stale config) */}
        <div key={selectedNode.id} className="px-4 py-3">
          {/* Read-only disables the forms only: buttons outside them (preview, messages) stay. */}
          <ConfigProvider componentDisabled={readOnly}>
            {/* Every node type takes a description, so it is edited here rather than repeated in
                each of the per-type forms below. It is stored in the node's config. */}
            <Form layout="vertical" className="nodrag">
              {/* htmlFor/id by hand: a Form.Item with no `name` generates no field id, so without
                  these the label is not associated with the input for assistive tech. */}
              <Form.Item label={t`Description`} htmlFor="node-description">
                <Input
                  id="node-description"
                  value={(config as { description?: string }).description || ''}
                  // undefined rather than '': clearing the box should drop the key instead of
                  // storing a blank. Emptiness is judged on the trimmed value, while the value
                  // itself is stored as typed.
                  onChange={(e) =>
                    writeConfig({
                      ...config,
                      description: e.target.value.trim() ? e.target.value : undefined
                    })
                  }
                  placeholder={t`e.g., Welcome — day 1`}
                  maxLength={100}
                />
              </Form.Item>
            </Form>
            {renderConfigForm()}
          </ConfigProvider>
        </div>
        {nodeType === 'email' ? (
          <section className="border-t border-line-soft px-4 py-3" data-step-preview>
            <EmailStepPreview workspace={workspace} config={config as Partial<EmailNodeConfig>} canReadTemplates={canReadTemplates} />
          </section>
        ) : null}
        {resultsBlock()}
        {nodeType !== 'trigger' && !readOnly && onDelete ? (
          <div className="border-t border-line-soft px-4 py-3">
            <Popconfirm
              title={t`Delete node`}
              description={t`Are you sure you want to delete this node?`}
              onConfirm={() => onDelete(selectedNode.id)}
              okText={t`Delete`}
              cancelText={t`Cancel`}
              okButtonProps={{ danger: true }}
            >
              <Button danger size="small" icon={<Trash2 size={14} aria-hidden="true" />}>
                {t`Delete step`}
              </Button>
            </Popconfirm>
          </div>
        ) : null}
      </div>
    </div>
  )
}
