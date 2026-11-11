import React from 'react'
import { Button, Tag, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock } from '../types'
import { BlockThumb } from './BlockThumb'
import { useLibraryDraggable, type LibraryItem } from '../utils/library'

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
  highlighted?: boolean
  onInsert: (item: LibraryItem) => void
}

const formatDate = (date: string | undefined, locale: string) => {
  if (!date) return null
  const parsed = new Date(date)
  if (Number.isNaN(parsed.getTime())) return null
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parsed)
  } catch {
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium' }).format(parsed)
  }
}

export const SavedBlockCard: React.FC<SavedBlockCardProps> = ({
  item,
  typeLabel,
  date,
  locale,
  emailTree,
  headKey,
  refusal,
  highlighted,
  onInsert
}) => {
  const { t } = useLingui()
  const { listeners, setNodeRef, isDragging } = useLibraryDraggable(item)
  const formatted = formatDate(date, locale)

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
        className="cursor-grab select-none"
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
            <span className="truncate text-xs font-medium text-gray-800" title={item.name}>
              {item.name}
            </span>
            {highlighted && (
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
        className="flex items-center justify-between gap-2 px-2 pb-1.5 pt-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100"
      >
        <span className="truncate text-[10px] text-gray-400">{t`or drag the block into the email`}</span>
        {/* A disabled button gets no pointer events: the tooltip listens on its wrapper */}
        <Tooltip title={refusal ?? undefined}>
          <span className="inline-block shrink-0">
            <Button size="small" disabled={!!refusal} onClick={() => onInsert(item)}>
              {t`Insert`}
            </Button>
          </span>
        </Tooltip>
      </div>
    </div>
  )
}
