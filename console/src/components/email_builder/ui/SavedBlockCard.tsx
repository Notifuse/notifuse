import React, { useEffect, useRef, useState } from 'react'
import { Button, Dropdown, Input, Tag, Tooltip, type InputRef, type MenuProps } from 'antd'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faEllipsis } from '@fortawesome/free-solid-svg-icons'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock } from '../types'
import { BlockThumb } from './BlockThumb'
import { formatSavedDate, useLibraryDraggable, type LibraryItem } from '../utils/library'
import { MAX_BLOCK_NAME_BYTES, fitUtf8, utf8Length } from '../utils/blockName'

// What a member who may change the library can do with a saved block
export interface CardActions {
  // A library change is running: one at a time, so every card waits
  busy: boolean
  // Why the selected block cannot replace this one; null when it can
  replaceRefusal: string | null
  // Resolves once renamed; rejects when the rename failed
  onRename?: (name: string) => Promise<void>
  onReplace: () => void
  onDuplicate: () => void
  onDelete?: () => void
}

interface SavedBlockCardProps {
  item: LibraryItem
  typeLabel: string
  // Modification date, else creation date
  date?: string
  locale: string
  emailTree: EmailBlock
  headKey: string
  // Why "Insert" cannot place the block, from the selected block; null when it can
  refusal: string | null
  // Just created ("New" tag) or just changed (border only)
  highlight?: 'new' | 'updated' | null
  onInsert: (item: LibraryItem) => void
  // Rename, replace, duplicate and delete; none when absent
  actions?: CardActions
}

export const SavedBlockCard: React.FC<SavedBlockCardProps> = ({
  item,
  typeLabel,
  date,
  locale,
  emailTree,
  headKey,
  refusal,
  highlight,
  onInsert,
  actions
}) => {
  const { t } = useLingui()
  const [menuOpen, setMenuOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(item.name)
  const [saving, setSaving] = useState(false)
  // Set once Enter, Escape or a blur has ended the edit, so the blur that follows does nothing
  const ended = useRef(false)
  const inputRef = useRef<InputRef>(null)
  const { listeners, setNodeRef, isDragging } = useLibraryDraggable(item, editing)
  const formatted = formatSavedDate(date, locale)
  const highlighted = !!highlight
  const name = item.name

  // Focused once the menu has closed, so its closing does not take the focus back
  useEffect(() => {
    if (editing && !menuOpen) inputRef.current?.focus({ cursor: 'all' })
  }, [editing, menuOpen])

  const startRename = () => {
    ended.current = false
    setDraft(item.name)
    setEditing(true)
  }

  const endRename = async (save: boolean) => {
    if (ended.current) return
    ended.current = true
    const name = draft.trim()
    if (!save || !name || name === item.name || !actions?.onRename) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await actions.onRename(name)
    } catch {
      // The handler reports the error; the old name stays
    } finally {
      setSaving(false)
      setEditing(false)
    }
  }

  const menu: MenuProps['items'] = actions
    ? [
        { key: 'rename', label: t`Rename`, disabled: actions.busy || !actions.onRename },
        {
          key: 'replace',
          disabled: actions.busy || !!actions.replaceRefusal,
          label: (
            <div>
              <div>{t`Replace with the selected block`}</div>
              {actions.replaceRefusal && (
                <div className="text-[11px] text-gray-400">{actions.replaceRefusal}</div>
              )}
            </div>
          )
        },
        { key: 'duplicate', label: t`Duplicate`, disabled: actions.busy },
        { type: 'divider' },
        {
          key: 'delete',
          label: t`Delete`,
          danger: true,
          disabled: actions.busy || !actions.onDelete
        }
      ]
    : []

  const onMenuClick: MenuProps['onClick'] = ({ key }) => {
    setMenuOpen(false)
    if (!actions) return
    if (key === 'rename') startRename()
    else if (key === 'replace') actions.onReplace()
    else if (key === 'duplicate') actions.onDuplicate()
    else if (key === 'delete') actions.onDelete?.()
  }

  return (
    <div
      data-library-card={item.key}
      className={`group rounded border bg-white transition-colors ${
        highlighted ? 'border-[#7763F1]' : 'border-gray-200 hover:border-[#7763F1]'
      }`}
      style={{ opacity: isDragging ? 0.4 : 1 }}
    >
      {/* The card's body is the drag handle: a plain element, so "Insert" is the only control in the card */}
      <div
        ref={setNodeRef}
        {...listeners}
        data-library-drag-handle
        className={editing ? '' : 'cursor-grab select-none'}
        style={{ touchAction: 'none' }}
      >
        <div className="border-b border-gray-100 overflow-hidden rounded-t">
          <BlockThumb
            block={item.block}
            emailTree={emailTree}
            headKey={headKey}
            width={286}
            maxHeight={120}
          />
        </div>
        <div className="px-2 pt-1.5">
          <div className="flex items-center gap-1">
            {editing ? (
              <Input
                ref={inputRef}
                size="small"
                aria-label={t`Block name`}
                value={draft}
                disabled={saving}
                count={{
                  max: MAX_BLOCK_NAME_BYTES,
                  strategy: utf8Length,
                  exceedFormatter: (text, { max }) => fitUtf8(text, max)
                }}
                onChange={(e) => setDraft(e.target.value)}
                onPressEnter={() => endRename(true)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') {
                    e.stopPropagation()
                    endRename(false)
                  }
                }}
                onBlur={() => endRename(true)}
              />
            ) : (
              <span className="truncate text-xs font-medium text-gray-800" title={item.name}>
                {item.name}
              </span>
            )}
            {highlight === 'new' && (
              <Tag color="purple" className="!m-0 !text-[10px] !leading-4">
                {t`New`}
              </Tag>
            )}
          </div>
          <div className="text-[11px] text-gray-500">
            {typeLabel}
            {formatted && <span> · {formatted}</span>}
          </div>
        </div>
      </div>
      {/* Hidden with opacity, not display or visibility, so "Insert" stays in the tab order */}
      <div
        data-testid="library-card-actions"
        className={`flex items-center justify-between gap-2 px-2 pb-1.5 pt-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100${
          menuOpen || editing ? ' !opacity-100' : ''
        }`}
      >
        <span className="truncate text-[10px] text-gray-400">{t`or drag the block into the email`}</span>
        <span className="flex shrink-0 items-center gap-1">
          {/* A disabled button gets no pointer events: the tooltip listens on its wrapper */}
          <Tooltip title={refusal ?? undefined}>
            <span className="inline-block shrink-0">
              <Button size="small" disabled={!!refusal} onClick={() => onInsert(item)}>
                {t`Insert`}
              </Button>
            </span>
          </Tooltip>
          {actions && (
            <Dropdown
              trigger={['click']}
              open={menuOpen}
              onOpenChange={setMenuOpen}
              menu={{ items: menu, onClick: onMenuClick }}
            >
              <Button
                size="small"
                type="text"
                className="shrink-0"
                aria-label={t`More actions for ${name}`}
                icon={<FontAwesomeIcon icon={faEllipsis} />}
              />
            </Dropdown>
          )}
        </span>
      </div>
    </div>
  )
}
