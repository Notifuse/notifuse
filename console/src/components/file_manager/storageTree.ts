import type { StorageObject } from './interfaces'

// Pure functions over the items of a bucket (toStorageItems), shared by the file manager page and
// the image picker. A folder is a key ending with "/", with or without a marker in the bucket.

/** Folders first, then the newest first: the order of every list of the file manager. */
export const byFoldersThenNewest = (a: StorageObject, b: StorageObject): number => {
  if (a.is_folder !== b.is_folder) return a.is_folder ? -1 : 1
  const left = a.last_modified ? new Date(a.last_modified).getTime() : 0
  const right = b.last_modified ? new Date(b.last_modified).getTime() : 0
  return right - left
}

/** The items directly inside a folder ("" is the root). */
export function childrenOf(items: StorageObject[], folder: string): StorageObject[] {
  return items.filter((item) => item.path === folder).sort(byFoldersThenNewest)
}

/** The number of files under each folder, sub-folders included; "" holds the bucket's total. */
export function fileCounts(items: StorageObject[]): Map<string, number> {
  const counts = new Map<string, number>([['', 0]])
  for (const item of items) {
    if (item.is_folder) {
      if (!counts.has(item.key)) counts.set(item.key, 0)
      continue
    }
    counts.set('', (counts.get('') ?? 0) + 1)
    for (const folder of ancestorFolders(item.path)) {
      counts.set(folder, (counts.get(folder) ?? 0) + 1)
    }
  }
  return counts
}

/** The size of the files, in bytes. */
export function totalSize(items: StorageObject[]): number {
  return items.reduce((sum, item) => sum + (item.is_folder ? 0 : item.file_info?.size || 0), 0)
}

export interface FolderNode {
  key: string
  name: string
  children: FolderNode[]
}

/** The folders as a tree, each level sorted by name. */
export function folderTree(items: StorageObject[]): FolderNode[] {
  const nodes = new Map<string, FolderNode>()
  for (const item of items) {
    if (item.is_folder) nodes.set(item.key, { key: item.key, name: item.name, children: [] })
  }
  const roots: FolderNode[] = []
  for (const node of nodes.values()) {
    const parent = nodes.get(parentFolder(node.key))
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  const sort = (level: FolderNode[]) => {
    level.sort((a, b) => a.name.localeCompare(b.name))
    level.forEach((node) => sort(node.children))
  }
  sort(roots)
  return roots
}

/** Files whose name contains the text, whatever the case, in the whole bucket. */
export function searchFiles(items: StorageObject[], text: string): StorageObject[] {
  const needle = text.trim().toLowerCase()
  if (needle === '') return []
  return items
    .filter((item) => !item.is_folder && item.name.toLowerCase().includes(needle))
    .sort(byFoldersThenNewest)
}

/** The folder above a folder: "a/b/" → "a/", "a/" → "". */
export function parentFolder(folder: string): string {
  const parts = folder.split('/').filter((part) => part !== '')
  return parts.length <= 1 ? '' : parts.slice(0, -1).join('/') + '/'
}

/** The folders from the root down to this one: "a/b/" → ["a/", "a/b/"]. */
export function ancestorFolders(folder: string): string[] {
  const parts = folder.split('/').filter((part) => part !== '')
  return parts.map((_, index) => parts.slice(0, index + 1).join('/') + '/')
}

/** The last segment of a folder: "a/b/" → "b"; "" for the root. */
export function folderName(folder: string): string {
  return folder.split('/').filter((part) => part !== '').pop() ?? ''
}
