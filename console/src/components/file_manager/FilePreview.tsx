import { useState } from 'react'
import { Button, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Copy, Download, ImageOff } from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { KeyValueList, Mono, useRelativeDate, type KeyValueItem } from '../layout'
import type { StorageObject } from './interfaces'
import { fileKind } from './fileKind'
import { FileTypeIcon } from './FileTypeIcon'
import { recordImageSize, useImageSize } from './imageDimensions'

export interface FilePreviewProps {
  item: StorageObject
  /** With a search: the file's folder, as a link that opens it. */
  showFolder?: boolean
  onOpenFolder: (folder: string) => void
  onCopyUrl: () => void
  onDownload: () => void
  /** The page's Delete link; absent in the picker. */
  onDelete?: () => void
  readOnly?: boolean
}

/** The chosen file: its thumbnail, name, details and actions. */
export function FilePreview({
  item,
  showFolder,
  onOpenFolder,
  onCopyUrl,
  onDownload,
  onDelete,
  readOnly
}: FilePreviewProps) {
  const { t } = useLingui()
  const relative = useRelativeDate()
  const isImage = fileKind(item) === 'image'
  const size = useImageSize(isImage ? item.file_info.url : undefined)
  // Keyed by URL: another file starts again from a fresh load.
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null)
  const broken = brokenUrl === item.file_info.url

  const details: KeyValueItem[] = [
    ...(isImage
      ? [{ label: t`Dimensions`, value: size ? `${size.width} × ${size.height}` : null }]
      : []),
    { label: t`Size`, value: item.file_info.size_human },
    { label: t`Type`, value: item.file_info.content_type ? <Mono>{item.file_info.content_type}</Mono> : null },
    {
      label: t`Modified`,
      value: item.last_modified ? (
        <Tooltip title={dayjs(item.last_modified).format('llll')}>
          <span>{relative(item.last_modified)}</span>
        </Tooltip>
      ) : null
    },
    ...(showFolder
      ? [
          {
            label: t`Folder`,
            value: (
              <Button type="link" size="small" onClick={() => onOpenFolder(item.path)}>
                {item.path || t`Root`}
              </Button>
            )
          }
        ]
      : [])
  ]

  return (
    <div data-file-preview className="flex flex-col gap-3">
      <div className="flex h-[200px] items-center justify-center overflow-hidden rounded-md bg-subtle">
        {isImage && !broken ? (
          <img
            src={item.file_info.url}
            alt={item.name}
            className="max-h-full max-w-full object-contain"
            onLoad={(event) => recordImageSize(item.file_info.url, event.currentTarget)}
            onError={() => setBrokenUrl(item.file_info.url)}
          />
        ) : isImage ? (
          <div className="flex flex-col items-center gap-2 text-sm text-ink-3">
            <ImageOff size={32} aria-hidden="true" />
            {t`This image couldn't be loaded`}
          </div>
        ) : (
          <FileTypeIcon item={item} size={48} />
        )}
      </div>
      <div className="text-base font-medium break-all text-ink">{item.name}</div>
      <KeyValueList items={details} labelWidth={90} />
      <div className="flex gap-2">
        <Button icon={<Copy size={14} aria-hidden="true" />} onClick={onCopyUrl}>
          {t`Copy URL`}
        </Button>
        <Button icon={<Download size={14} aria-hidden="true" />} onClick={onDownload}>
          {t`Download`}
        </Button>
      </div>
      <div className="flex items-center gap-4 text-sm">
        <a href={item.file_info.url} target="_blank" rel="noreferrer">
          {t`Open in a new tab`}
        </a>
        {onDelete ? (
          readOnly ? (
            <Tooltip title={t`Delete file (Read-only mode)`}>
              <Button type="link" size="small" danger disabled>
                {t`Delete`}
              </Button>
            </Tooltip>
          ) : (
            <Button type="link" size="small" danger onClick={onDelete}>
              {t`Delete`}
            </Button>
          )
        ) : null}
      </div>
    </div>
  )
}
