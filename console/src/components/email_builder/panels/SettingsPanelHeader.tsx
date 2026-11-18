import { Fragment } from 'react'
import { Popconfirm, Tooltip } from 'antd'
import { Copy, Save, Trash2 } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { blockPath, headerActions, headerTone, type HeaderTone } from './settingsPanelHeaderModel'

export interface SettingsPanelHeaderProps {
  block: EmailBlock
  emailTree: EmailBlock
  /** Selects an ancestor clicked in the path. */
  onSelectBlock?: (blockId: string) => void
  onCloneBlock?: (blockId: string) => void
  onDeleteBlock?: (blockId: string) => void
  /** Opens the editor's save-to-library dialog for the block. */
  onSaveBlock?: (blockId: string) => void
}

const TONE: Record<HeaderTone, string> = {
  content: 'bg-primary-100 text-primary-700',
  structure: 'bg-info-bg text-info-fg',
  head: 'bg-fill text-ink-2',
  code: 'bg-warning-bg text-warning-fg'
}

const ICON_BUTTON =
  'flex size-6.5 flex-none cursor-pointer items-center justify-center rounded-md border-0 bg-transparent p-0 hover:bg-fill'

// The top of the settings panel: the block's icon and name, its path in the tree (ancestors
// select on click), and the actions the canvas toolbar offers (clone, save to library, delete).
export function SettingsPanelHeader({
  block,
  emailTree,
  onSelectBlock,
  onCloneBlock,
  onDeleteBlock,
  onSaveBlock
}: SettingsPanelHeaderProps) {
  const { t } = useLingui()
  const found = blockPath(emailTree, block.id)
  const path = found.length > 0 ? found : [block]
  const actions = headerActions(path)
  const labelOf = (b: EmailBlock) => new EmailBlockClass(b).getLabel()
  const parentType = path.at(-2)?.type

  return (
    <div className="flex items-center gap-2.5 border-b border-line bg-surface px-3.5 py-3" data-testid="settings-panel-header">
      <span
        data-testid="settings-panel-header-icon"
        className={`flex size-7.5 flex-none items-center justify-center rounded-[7px] ${TONE[headerTone(path)]}`}
        aria-hidden="true"
      >
        {new EmailBlockClass(block).getIcon(parentType)}
      </span>
      <div className="min-w-0">
        <div className="truncate text-sm/tight font-semibold text-ink">{labelOf(block)}</div>
        {path.length > 1 ? (
          <nav aria-label={t`Block path`} className="mt-0.5 flex flex-wrap items-center gap-x-1 gap-y-0.5 text-2xs text-ink-3">
            {path.map((step, index) => {
              const last = index === path.length - 1
              return (
                <Fragment key={step.id}>
                  {last ? (
                    <span aria-current="location">{labelOf(step)}</span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => onSelectBlock?.(step.id)}
                      className="cursor-pointer border-0 border-b border-dotted border-ink-4 bg-transparent p-0 text-2xs text-ink-3 hover:text-ink"
                    >
                      {labelOf(step)}
                    </button>
                  )}
                  {last ? null : (
                    <span aria-hidden="true" className="text-ink-4">
                      ›
                    </span>
                  )}
                </Fragment>
              )
            })}
          </nav>
        ) : null}
      </div>
      {actions.clone || actions.save || actions.remove ? (
        <span className="ml-auto flex flex-none gap-0.5">
          {actions.clone && onCloneBlock ? (
            <Tooltip title={t`Clone Block`} placement="bottom">
              <button type="button" aria-label={t`Clone Block`} className={ICON_BUTTON} onClick={() => onCloneBlock(block.id)}>
                <Copy size={14} aria-hidden="true" className="text-ink-3" />
              </button>
            </Tooltip>
          ) : null}
          {actions.save && onSaveBlock ? (
            <Tooltip title={t`Save to library`} placement="bottom">
              <button type="button" aria-label={t`Save to library`} className={ICON_BUTTON} onClick={() => onSaveBlock(block.id)}>
                <Save size={14} aria-hidden="true" className="text-ink-3" />
              </button>
            </Tooltip>
          ) : null}
          {actions.remove && onDeleteBlock ? (
            <Popconfirm
              title={t`Are you sure you want to delete this block?`}
              placement="bottomRight"
              onConfirm={() => onDeleteBlock(block.id)}
            >
              <Tooltip title={t`Delete Block`} placement="bottom">
                <button type="button" aria-label={t`Delete Block`} className={ICON_BUTTON}>
                  <Trash2 size={14} aria-hidden="true" className="text-ink-3" />
                </button>
              </Tooltip>
            </Popconfirm>
          ) : null}
        </span>
      ) : null}
    </div>
  )
}
