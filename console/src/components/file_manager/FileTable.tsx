import type { Key, ReactNode } from 'react'
import { Button, Dropdown, Table, Tooltip } from 'antd'
import type { MenuProps, TableColumnType } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { MoreHorizontal } from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { useRelativeDate } from '../layout'
import type { StorageObject } from './interfaces'
import { fileKind } from './fileKind'
import { FileTypeIcon } from './FileTypeIcon'
import { recordImageSize, useImageSize } from './imageDimensions'

export interface FileTableProps {
  items: StorageObject[]
  loading?: boolean
  selectedKey?: string
  /** Checkboxes on the files (the page); none in the picker. */
  checkedKeys?: string[]
  onCheckedChange?: (keys: string[]) => void
  isRefused?: (item: StorageObject) => boolean
  counts: Map<string, number>
  /** Search results: each file shows its folder under its name. */
  showFolder?: boolean
  emptyText: string
  onOpen: (item: StorageObject) => void
  onActivate?: (item: StorageObject) => void
  menuFor: (item: StorageObject) => MenuProps['items']
}

/** "1200 × 800" once the image has loaded, read from a hidden lazy thumbnail. */
function Dimensions({ item }: { item: StorageObject }) {
  const url = fileKind(item) === 'image' ? item.file_info.url : undefined
  const size = useImageSize(url)
  if (!url) return <span className="text-ink-4">—</span>
  return (
    <span className="relative tabular-nums">
      {size ? `${size.width} × ${size.height}` : <span className="text-ink-4">—</span>}
      {size ? null : (
        <img
          src={url}
          alt=""
          loading="lazy"
          aria-hidden="true"
          className="pointer-events-none absolute top-0 left-0 size-px opacity-0"
          onLoad={(event) => recordImageSize(url, event.currentTarget)}
        />
      )}
    </span>
  )
}

/** The items of a folder, or the files a search found, as a table. */
export function FileTable({
  items,
  loading,
  selectedKey,
  checkedKeys,
  onCheckedChange,
  isRefused,
  counts,
  showFolder,
  emptyText,
  onOpen,
  onActivate,
  menuFor
}: FileTableProps) {
  const { t } = useLingui()
  const relative = useRelativeDate()

  // A file the caller cannot use is dimmed; the classes go on the cell content, not on antd's row.
  const refused = (item: StorageObject) => !item.is_folder && Boolean(isRefused?.(item))
  const cell = (item: StorageObject, content: ReactNode) =>
    refused(item) ? <span className="opacity-50">{content}</span> : content

  const columns: TableColumnType<StorageObject>[] = [
    {
      title: t`Name`,
      key: 'name',
      render: (_, item) => (
        <div className={`flex min-w-0 items-center gap-2.5 ${refused(item) ? 'opacity-50' : ''}`}>
          <FileTypeIcon item={item} />
          <div className="flex min-w-0 flex-col">
            <span className={`truncate ${item.is_folder ? 'font-medium text-ink' : 'text-ink'}`}>
              {item.name}
            </span>
            {showFolder ? (
              <span className="truncate text-xs text-ink-3">{item.path || t`Root`}</span>
            ) : null}
          </div>
        </div>
      )
    },
    {
      title: t`Dimensions`,
      key: 'dimensions',
      width: 130,
      render: (_, item) => cell(item, <Dimensions item={item} />)
    },
    {
      title: t`Size`,
      key: 'size',
      width: 110,
      render: (_, item) => {
        if (!item.is_folder) return cell(item, <span className="tabular-nums">{item.file_info.size_human}</span>)
        const count = counts.get(item.key) ?? 0
        return (
          <span className="text-ink-3 tabular-nums">
            {plural(count, { one: '# file', other: '# files' })}
          </span>
        )
      }
    },
    {
      title: t`Modified`,
      key: 'modified',
      width: 150,
      render: (_, item) =>
        item.implicit || !item.last_modified ? (
          <span className="text-ink-4">—</span>
        ) : (
          cell(
            item,
            <Tooltip title={dayjs(item.last_modified).format('llll')}>
              <span className="text-ink-2">{relative(item.last_modified)}</span>
            </Tooltip>
          )
        )
    },
    {
      title: '',
      key: 'actions',
      width: 48,
      align: 'right',
      render: (_, item) => {
        const menu = menuFor(item)
        if (!menu || menu.length === 0) return null
        return (
          <span onClick={(event) => event.stopPropagation()}>
            <Dropdown menu={{ items: menu }} trigger={['click']} placement="bottomRight">
              <Button
                type="text"
                size="small"
                aria-label={t`Actions for ${item.name}`}
                icon={<MoreHorizontal size={16} aria-hidden="true" />}
              />
            </Dropdown>
          </span>
        )
      }
    }
  ]

  return (
    <Table<StorageObject>
      dataSource={items}
      columns={columns}
      rowKey="key"
      loading={loading}
      pagination={false}
      size="middle"
      locale={{ emptyText }}
      rowSelection={
        checkedKeys && onCheckedChange
          ? {
              type: 'checkbox',
              selectedRowKeys: checkedKeys,
              onChange: (keys: Key[]) => onCheckedChange(keys.map(String)),
              getCheckboxProps: (item: StorageObject) => ({
                disabled: item.is_folder,
                'aria-label': item.name
              })
            }
          : undefined
      }
      onRow={(item) => {
        const isRefusedRow = refused(item)
        return {
          onClick: isRefusedRow ? undefined : () => onOpen(item),
          onDoubleClick:
            isRefusedRow || item.is_folder || !onActivate ? undefined : () => onActivate(item),
          'aria-selected': item.key === selectedKey,
          'data-selected': item.key === selectedKey ? 'true' : undefined,
          title: isRefusedRow ? t`This file type can't be used here` : undefined,
          style: { cursor: isRefusedRow ? 'not-allowed' : 'pointer' }
        }
      }}
      rowClassName={(item) => (item.key === selectedKey ? 'ant-table-row-selected' : '')}
    />
  )
}
