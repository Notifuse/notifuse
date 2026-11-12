import React from 'react'
import { Trash2 } from 'lucide-react'
import { Tooltip, Popconfirm } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../../services/api/automation'
import { NODE_TYPES } from '../nodeTypes'
import { NodeStatsFooter } from '../stats/NodeStatsFooter'

interface BaseNodeProps {
  type: NodeType
  /** The step's id: its results come under the card with "With stats" on. */
  nodeId?: string
  /** Optional author-written note, the card's title so the flow reads without opening nodes. */
  description?: string
  selected?: boolean
  isOrphan?: boolean
  /** The step's detail line: what it does, as configured. */
  children?: React.ReactNode
  onDelete?: () => void
}

/** A step on the canvas: icon chip and type, the author's title, the detail, then the results. */
export const BaseNode: React.FC<BaseNodeProps> = ({ type, nodeId, description, selected, isOrphan, children, onDelete }) => {
  const { t, i18n } = useLingui()
  const meta = NODE_TYPES[type]
  const Icon = meta.icon
  const border = selected ? 'border-primary-500 shadow-md' : isOrphan ? 'border-warning-fg' : 'border-line'

  return (
    <div className="relative">
      {isOrphan && (
        <div className="absolute -top-6 right-0 left-0 text-center text-xs font-medium text-warning-fg">
          {t`Not connected`}
        </div>
      )}
      {selected && type !== 'trigger' && onDelete && (
        <div className="absolute top-1/2 -right-8 z-10 -translate-y-1/2">
          <Popconfirm
            title={t`Delete node`}
            description={t`Are you sure you want to delete this node?`}
            onConfirm={onDelete}
            okText={t`Delete`}
            cancelText={t`Cancel`}
            okButtonProps={{ danger: true }}
          >
            <Tooltip title={t`Delete node`} placement="right">
              <button
                type="button"
                aria-label={t`Delete node`}
                className="flex size-6 cursor-pointer items-center justify-center rounded-full border border-line bg-surface text-ink-4 shadow-md transition-transform hover:scale-110 hover:text-danger-fg"
              >
                <Trash2 size={14} aria-hidden="true" />
              </button>
            </Tooltip>
          </Popconfirm>
        </div>
      )}
      <div
        data-automation-node
        className={`automation-node w-75 rounded-lg border bg-surface px-3 py-2.5 ${border}`}
      >
        <div className="flex items-center gap-2">
          <span className={`flex size-5.5 shrink-0 items-center justify-center rounded-md ${meta.chip}`}>
            <Icon size={12} aria-hidden="true" />
          </span>
          <span className="text-xs font-medium tracking-wide text-ink-3 uppercase">{i18n._(meta.label)}</span>
        </div>
        {description && (
          // Truncated to keep every card the same height; the full text stays reachable on hover.
          <div className="mt-1.5 truncate text-sm font-medium text-ink" title={description}>
            {description}
          </div>
        )}
        {children && <div className={`text-sm text-ink-2 ${description ? 'mt-0.5' : 'mt-1.5'}`}>{children}</div>}
        {nodeId ? <NodeStatsFooter nodeId={nodeId} nodeType={type} /> : null}
      </div>
    </div>
  )
}
