import React from 'react'
import { Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { NodeType } from '../../services/api/automation'
import { PALETTE_GROUPS, NODE_TYPES } from './nodeTypes'
import { NodeTypeChip } from './AddNodeButton'

interface NodePaletteProps {
  hasListSelected: boolean
}

/** The steps to drag onto the canvas, in two groups. The editor leaves it out read-only. */
export const NodePalette: React.FC<NodePaletteProps> = ({ hasListSelected }) => {
  const { t, i18n } = useLingui()

  const onDragStart = (event: React.DragEvent, nodeType: NodeType) => {
    event.dataTransfer.setData('application/reactflow', nodeType)
    event.dataTransfer.effectAllowed = 'move'
  }

  return (
    <div className="flex flex-col gap-3" data-node-palette>
      {PALETTE_GROUPS.map((group) => (
        <section key={group.key} className="flex flex-col gap-1.5" aria-label={i18n._(group.title)}>
          <h3 data-unreset className="text-xs font-medium tracking-wide text-ink-3 uppercase">
            {i18n._(group.title)}
          </h3>
          {group.types.map((type) => {
            const isDisabled = type === 'email' && !hasListSelected
            const item = (
              <div
                key={type}
                data-palette-item={type}
                aria-disabled={isDisabled || undefined}
                className={`flex items-center gap-2 rounded-md border border-line bg-surface px-2.5 py-1.5 text-sm text-ink ${
                  isDisabled ? 'cursor-not-allowed opacity-50' : 'cursor-grab hover:border-line hover:bg-subtle'
                }`}
                draggable={!isDisabled}
                onDragStart={(e) => !isDisabled && onDragStart(e, type)}
              >
                <NodeTypeChip type={type} />
                <span>{i18n._(NODE_TYPES[type].label)}</span>
              </div>
            )
            return isDisabled ? (
              <Tooltip key={type} title={t`Select a list to enable email nodes`} placement="right">
                {item}
              </Tooltip>
            ) : (
              item
            )
          })}
        </section>
      ))}
      <p className="text-xs text-ink-3">{t`Drag a step onto the canvas, or use + under a step.`}</p>
    </div>
  )
}
