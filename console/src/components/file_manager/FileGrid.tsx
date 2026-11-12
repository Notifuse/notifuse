import type { KeyboardEvent } from 'react'
import { Button, Dropdown, Tooltip } from 'antd'
import type { MenuProps } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { MoreHorizontal } from 'lucide-react'
import type { StorageObject } from './interfaces'
import { fileKind } from './fileKind'
import { FileTypeIcon, MiddleTruncate } from './FileTypeIcon'
import { recordImageSize, useImageSize } from './imageDimensions'

export interface FileGridProps {
  items: StorageObject[]
  selectedKey?: string
  /** Files the caller cannot use: dimmed and not selectable. */
  isRefused?: (item: StorageObject) => boolean
  /** A click: opens a folder, selects a file. */
  onOpen: (item: StorageObject) => void
  /** A double click or Enter on a file (the picker inserts it). */
  onActivate?: (item: StorageObject) => void
  menuFor: (item: StorageObject) => MenuProps['items']
}

function Card({
  item,
  selected,
  refused,
  onOpen,
  onActivate,
  menu
}: {
  item: StorageObject
  selected: boolean
  refused: boolean
  onOpen: () => void
  onActivate?: () => void
  menu: MenuProps['items']
}) {
  const { t } = useLingui()
  const kind = fileKind(item)
  const size = useImageSize(kind === 'image' ? item.file_info.url : undefined)

  const caption =
    kind === 'folder'
      ? t`Folder`
      : size
        ? `${size.width} × ${size.height}`
        : (item.file_info?.size_human ?? '')

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) return
    if (event.key === 'Enter') {
      event.preventDefault()
      if (item.is_folder || !onActivate) onOpen()
      else onActivate()
    } else if (event.key === ' ') {
      event.preventDefault()
      onOpen()
    }
  }

  const card = (
    <div
      role="button"
      tabIndex={refused ? -1 : 0}
      aria-pressed={item.is_folder ? undefined : selected}
      aria-disabled={refused || undefined}
      aria-label={item.name}
      data-file-card={item.key}
      onClick={refused ? undefined : onOpen}
      onDoubleClick={refused || item.is_folder ? undefined : onActivate}
      onKeyDown={refused ? undefined : handleKeyDown}
      className={`group relative flex min-w-0 cursor-pointer flex-col overflow-hidden rounded-lg border-2 bg-surface focus-visible:ring-2 focus-visible:ring-primary-500/18 focus-visible:outline-none ${
        selected ? 'border-primary' : 'border-line hover:border-gray-300'
      } ${refused ? 'cursor-not-allowed opacity-50' : ''}`}
    >
      <div className="flex aspect-4/3 items-center justify-center overflow-hidden bg-subtle">
        {kind === 'image' ? (
          <img
            src={item.file_info.url}
            alt=""
            loading="lazy"
            className="size-full object-cover"
            onLoad={(event) => recordImageSize(item.file_info.url, event.currentTarget)}
          />
        ) : (
          <FileTypeIcon item={item} size={32} />
        )}
      </div>
      <div className="flex min-w-0 flex-col gap-0.5 px-2.5 py-2">
        <span className={`text-sm ${item.is_folder ? 'font-medium text-ink' : 'text-ink'}`}>
          <MiddleTruncate text={item.name} />
        </span>
        <span className="truncate text-xs text-ink-3">{caption}</span>
      </div>
      {menu && menu.length > 0 ? (
        <span
          className="absolute top-1.5 right-1.5 rounded-md bg-surface opacity-0 shadow-card group-focus-within:opacity-100 group-hover:opacity-100"
          onClick={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()}
        >
          <Dropdown menu={{ items: menu }} trigger={['click']} placement="bottomRight">
            <Button
              type="text"
              size="small"
              aria-label={t`Actions for ${item.name}`}
              icon={<MoreHorizontal size={16} aria-hidden="true" />}
            />
          </Dropdown>
        </span>
      ) : null}
    </div>
  )

  return refused ? <Tooltip title={t`This file type can't be used here`}>{card}</Tooltip> : card
}

/** The items of a folder as cards of 180px or more, with a thumbnail for images. */
export function FileGrid({ items, selectedKey, isRefused, onOpen, onActivate, menuFor }: FileGridProps) {
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-3 p-3">
      {items.map((item) => (
        <Card
          key={item.key}
          item={item}
          selected={item.key === selectedKey}
          refused={!item.is_folder && Boolean(isRefused?.(item))}
          onOpen={() => onOpen(item)}
          onActivate={onActivate ? () => onActivate(item) : undefined}
          menu={menuFor(item)}
        />
      ))}
    </div>
  )
}
