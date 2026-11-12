import type { StorageObject } from './interfaces'

export type FileKind = 'folder' | 'image' | 'pdf' | 'other'

export function fileKind(item: StorageObject): FileKind {
  if (item.is_folder) return 'folder'
  const type = item.file_info?.content_type ?? ''
  if (type.startsWith('image/')) return 'image'
  if (type === 'application/pdf') return 'pdf'
  return 'other'
}
