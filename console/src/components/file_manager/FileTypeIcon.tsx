import { File, FileImage, FileText, Folder } from 'lucide-react'
import type { StorageObject } from './interfaces'
import { fileKind, type FileKind } from './fileKind'

// Whole class names: Tailwind only generates the classes it finds written out in the source.
const TONES: Record<FileKind, string> = {
  folder: 'text-primary-500',
  image: 'text-info-fg',
  pdf: 'text-danger-fg',
  other: 'text-ink-3'
}

const ICONS = { folder: Folder, image: FileImage, pdf: FileText, other: File } as const

/** The icon of a file manager item, coloured by its type. */
export function FileTypeIcon({ item, size = 16 }: { item: StorageObject; size?: number }) {
  const kind = fileKind(item)
  const Icon = ICONS[kind]
  return (
    <span aria-hidden="true" className={`flex shrink-0 items-center ${TONES[kind]}`}>
      <Icon size={size} />
    </span>
  )
}

/** A name cut in its middle when it is too long, so the extension stays readable. */
export function MiddleTruncate({ text }: { text: string }) {
  const tail = text.length > 12 ? text.slice(-8) : ''
  const head = tail ? text.slice(0, -8) : text
  return (
    <span title={text} className="flex min-w-0">
      <span className="truncate">{head}</span>
      {tail ? <span className="shrink-0 whitespace-pre">{tail}</span> : null}
    </span>
  )
}
