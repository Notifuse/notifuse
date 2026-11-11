import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Input, Segmented, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, MJMLComponentType, SavedBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { EmailBlockFactory } from '../blocks/EmailBlockFactory'
import { SavedBlockCard, type CardActions } from '../ui/SavedBlockCard'
import { resolveInsertTarget } from '../utils/insertTarget'
import {
  headKeyOf,
  savedBlockTime,
  typeLabelOf,
  useInsertRefusal,
  useLibraryDraggable,
  type LibraryItem
} from '../utils/library'

type Filter = 'all' | 'sections' | 'content'

const SECTION_TYPES: MJMLComponentType[] = ['mj-section', 'mj-wrapper']

// The block types a body or a column offers, layout first
const BASIC_TYPES: MJMLComponentType[] = [
  'mj-section',
  'mj-wrapper',
  'mj-text',
  'mj-image',
  'mj-button',
  'mj-divider',
  'mj-spacer',
  'mj-social',
  'mj-raw',
  'mj-liquid'
]

// Case and accents do not count in a search
const searchable = (value: string) => value.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

interface BasicTileProps {
  item: LibraryItem
  icon: React.ReactNode
  refusal: string | null
  onInsert: (item: LibraryItem) => void
}

// A basic block: a click inserts it, a drag carries it onto the email. It stays draggable when "Insert" cannot place
// it, since a drag does not depend on the selection.
const BasicTile: React.FC<BasicTileProps> = ({ item, icon, refusal, onInsert }) => {
  const { t } = useLingui()
  const { listeners, setNodeRef, isDragging } = useLibraryDraggable(item)
  return (
    <Tooltip title={refusal ?? t`Insert, or drag into the email`} mouseEnterDelay={0.4}>
      <button
        ref={setNodeRef}
        type="button"
        {...listeners}
        data-library-tile={item.block.type}
        aria-disabled={refusal ? true : undefined}
        onClick={() => {
          if (!refusal) onInsert(item)
        }}
        className={`flex cursor-grab select-none flex-col items-center gap-1 rounded border border-gray-200 bg-white px-1 py-2 text-[11px] text-gray-700 hover:border-[#7763F1] ${
          refusal ? 'text-gray-400' : ''
        }`}
        style={{ opacity: isDragging ? 0.4 : 1, touchAction: 'none' }}
      >
        <span className="text-sm">{icon}</span>
        <span className="truncate max-w-full">{item.name}</span>
      </button>
    </Tooltip>
  )
}

// A saved block just created ("new") or just changed ("updated") from this editor
export interface LibraryHighlight {
  id: string
  kind: 'new' | 'updated'
}

// What a member who may change the library can do with its saved blocks
export interface LibraryManage {
  // A library change is running
  busy: boolean
  onRename?: (saved: SavedBlock, name: string) => Promise<void>
  onReplace: (saved: SavedBlock) => void
  onDuplicate: (saved: SavedBlock) => void
  onDelete?: (saved: SavedBlock) => void
}

interface LibraryPanelProps {
  emailTree: EmailBlock
  savedBlocks: SavedBlock[]
  selectedBlockId: string | null
  highlight?: LibraryHighlight | null
  onInsert: (item: LibraryItem) => void
  onOpenSaveDialog?: (blockId: string) => void
  // Rename, replace, duplicate and delete on each card; none when absent
  manage?: LibraryManage
}

export const LibraryPanel: React.FC<LibraryPanelProps> = ({
  emailTree,
  savedBlocks,
  selectedBlockId,
  highlight,
  onInsert,
  onOpenSaveDialog,
  manage
}) => {
  const { t, i18n } = useLingui()
  const refusalText = useInsertRefusal()
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const headKey = useMemo(() => headKeyOf(emailTree), [emailTree])

  const items = useMemo(() => {
    const query = searchable(search.trim())
    return [...savedBlocks]
      .filter((saved) => {
        const isSection = SECTION_TYPES.includes(saved.block.type)
        if (filter === 'sections' && !isSection) return false
        if (filter === 'content' && isSection) return false
        return !query || searchable(saved.name).includes(query)
      })
      .sort((a, b) => savedBlockTime(b) - savedBlockTime(a))
  }, [savedBlocks, search, filter])

  const basics = useMemo(
    () =>
      BASIC_TYPES.filter((type) => EmailBlockFactory.hasBlockType(type)).map((type) => {
        const block = { id: `basic-${type}`, type, attributes: {} } as unknown as EmailBlock
        const instance = EmailBlockFactory.createBlock(block)
        return {
          item: {
            kind: 'basic',
            key: `basic-${type}`,
            name: instance.getLabel(),
            block
          } as LibraryItem,
          icon: instance.getIcon()
        }
      }),
    []
  )

  const refusalFor = (block: EmailBlock) =>
    refusalText(resolveInsertTarget(emailTree, selectedBlockId, block))

  const selected = selectedBlockId
    ? EmailBlockClass.findBlockById(emailTree, selectedBlockId)
    : null
  const selectedInBody =
    !!selected &&
    selected.type !== 'mj-body' &&
    EmailBlockClass.isChildOf(emailTree, selected.id, 'mj-body')
  const canSaveSelected = !!onOpenSaveDialog && selectedInBody

  // Only a block of the email's body, of the saved block's type, can replace it
  const replaceRefusal = (saved: SavedBlock) => {
    if (!selectedInBody) return t`Select a block in the email`
    if (selected!.type !== saved.block.type) return t`Select a block of the same type`
    return null
  }

  const actionsFor = (saved: SavedBlock): CardActions | undefined =>
    manage && {
      busy: manage.busy,
      replaceRefusal: replaceRefusal(saved),
      onRename: manage.onRename && ((name: string) => manage.onRename!(saved, name)),
      onReplace: () => manage.onReplace(saved),
      onDuplicate: () => manage.onDuplicate(saved),
      onDelete: manage.onDelete && (() => manage.onDelete!(saved))
    }

  // A changed block moves to the top of the list, sorted by modification date: it is brought into view
  const rootRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!highlight) return
    const card = rootRef.current?.querySelector(
      `[data-library-card=${JSON.stringify(highlight.id)}]`
    )
    if (card && typeof card.scrollIntoView === 'function') card.scrollIntoView({ block: 'nearest' })
  }, [highlight, savedBlocks])

  return (
    <div ref={rootRef} className="px-4 pb-8" data-testid="library-panel">
      {savedBlocks.length > 0 ? (
        <>
          <Input
            allowClear
            size="small"
            className="!mb-3"
            placeholder={t`Search saved blocks`}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="mb-2 flex items-center justify-between">
            <div className="text-sm font-bold text-gray-900">
              {t`Saved blocks`} <span className="font-normal text-gray-500">({items.length})</span>
            </div>
            <Segmented
              size="small"
              value={filter}
              onChange={(value) => setFilter(value as Filter)}
              options={[
                { label: t`All`, value: 'all' },
                { label: t`Sections`, value: 'sections' },
                { label: t`Content`, value: 'content' }
              ]}
            />
          </div>
          <div className="flex flex-col gap-2" data-testid="library-saved-blocks">
            {items.map((saved) => (
              <SavedBlockCard
                key={saved.id}
                item={{ kind: 'saved', key: saved.id, name: saved.name, block: saved.block }}
                typeLabel={typeLabelOf(saved.block)}
                date={saved.updated || saved.created}
                locale={i18n.locale}
                emailTree={emailTree}
                headKey={headKey}
                refusal={refusalFor(saved.block)}
                highlight={saved.id === highlight?.id ? highlight.kind : null}
                onInsert={onInsert}
                actions={actionsFor(saved)}
              />
            ))}
            {items.length === 0 && (
              <div className="py-4 text-center text-xs text-gray-500">{t`No saved block matches.`}</div>
            )}
          </div>
        </>
      ) : (
        <div
          className="rounded border border-dashed border-gray-300 bg-white p-4 text-xs text-gray-600"
          data-testid="library-empty"
        >
          <div className="mb-1 text-sm font-bold text-gray-900">{t`No saved blocks yet`}</div>
          <p className="mb-3">
            {t`Select a block in the email, then use the save button in its toolbar. Saved blocks can be used in every template of the workspace.`}
          </p>
          {canSaveSelected && (
            <Button size="small" type="primary" onClick={() => onOpenSaveDialog!(selected!.id)}>
              {t`Save the selected block`}
            </Button>
          )}
        </div>
      )}

      <div className="mb-2 mt-6 text-sm font-bold text-gray-900">{t`Basic blocks`}</div>
      <div className="grid grid-cols-3 gap-1.5" data-testid="library-basic-blocks">
        {basics.map(({ item, icon }) => (
          <BasicTile
            key={item.key}
            item={item}
            icon={icon}
            refusal={refusalFor(item.block)}
            onInsert={onInsert}
          />
        ))}
      </div>
    </div>
  )
}
