import { useState } from 'react'
import { Button, Tooltip } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { Copy, Download, ExternalLink, ImageOff, Trash2 } from 'lucide-react'
import dayjs from '../../lib/dayjs'
import { KeyValueList, Mono, useRelativeDate, type KeyValueItem } from '../layout'
import type { StorageObject } from './interfaces'
import { fileKind } from './fileKind'
import { FileTypeIcon } from './FileTypeIcon'
import { recordImageSize, useImageSize } from './imageDimensions'
import { PopconfirmWithTooltip } from '../layout/PopconfirmWithTooltip'

export interface FilePreviewProps {
  item: StorageObject
  /** With a search: the file's folder, as a link that opens it. */
  showFolder?: boolean
  onOpenFolder: (folder: string) => void
  onCopyUrl: () => void
  onDownload: () => void
  /** Deletes the file once its Popconfirm is accepted; absent in the picker. */
  onDelete?: () => Promise<void> | void
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
      <div className="flex flex-wrap gap-2">
        <Button icon={<Copy size={14} aria-hidden="true" />} onClick={onCopyUrl}>
          {t`Copy URL`}
        </Button>
        <Button icon={<Download size={14} aria-hidden="true" />} onClick={onDownload}>
          {t`Download`}
        </Button>
        <Tooltip title={t`Open in a new tab`}>
          <Button
            href={item.file_info.url}
            target="_blank"
            rel="noreferrer"
            aria-label={t`Open in a new tab`}
            icon={<ExternalLink size={14} aria-hidden="true" />}
          />
        </Tooltip>
        {onDelete ? (
          readOnly ? (
            <Tooltip title={t`Delete file (Read-only mode)`}>
              {/* A disabled button gets no pointer events: the tooltip listens on its wrapper */}
              <span className="inline-flex">
                <Button
                  danger
                  disabled
                  aria-label={t`Delete`}
                  icon={<Trash2 size={14} aria-hidden="true" />}
                />
              </span>
            </Tooltip>
          ) : (
            <PopconfirmWithTooltip
              tooltip={t`Delete`}
              title={t`Do you want to permanently delete this file from your storage?`}
              okText={t`Delete`}
              cancelText={t`Cancel`}
              okButtonProps={{ danger: true }}
              placement="bottomRight"
              onConfirm={onDelete}
            >
              <Button danger aria-label={t`Delete`} icon={<Trash2 size={14} aria-hidden="true" />} />
            </PopconfirmWithTooltip>
          )
        ) : null}
      </div>
    </div>
  )
}
