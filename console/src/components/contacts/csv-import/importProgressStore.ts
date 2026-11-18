import type { ColumnMappings } from './contactFields'

/** The key prefix of 43.x and earlier, kept so that an import saved before this version is offered. */
const PREFIX = 'csv_upload_progress_'
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000

/** Where an import stopped, kept in this browser so the same file can pick up from there. */
export interface SavedImport {
  fileName: string
  /** Index of the next data row to send (0 = the first row under the header). */
  currentRow: number
  totalRows: number
  currentBatch: number
  totalBatches: number
  mappings: ColumnMappings
  selectedListIds: string[]
  timestamp: number
}

export function importProgressKey(workspaceId: string, fileName: string): string {
  return `${PREFIX}${workspaceId}_${fileName}`
}

function parse(raw: string | null, now: number): SavedImport | null {
  if (!raw) return null
  try {
    const saved = JSON.parse(raw) as SavedImport
    const valid =
      !!saved &&
      typeof saved.fileName === 'string' &&
      typeof saved.currentRow === 'number' &&
      typeof saved.totalRows === 'number' &&
      typeof saved.currentBatch === 'number' &&
      typeof saved.totalBatches === 'number' &&
      typeof saved.mappings === 'object' &&
      saved.mappings !== null &&
      Array.isArray(saved.selectedListIds) &&
      typeof saved.timestamp === 'number'
    if (!valid) return null
    if (now - saved.timestamp >= MAX_AGE_MS) return null
    // A finished import is no longer one to resume.
    if (saved.currentRow < 0 || saved.currentRow >= saved.totalRows) return null
    return saved
  } catch {
    return null
  }
}

/** The import of this file saved in this workspace, if it can still be resumed. */
export function readImport(workspaceId: string, fileName: string, now = Date.now()): SavedImport | null {
  try {
    const saved = parse(localStorage.getItem(importProgressKey(workspaceId, fileName)), now)
    return saved && saved.fileName === fileName ? saved : null
  } catch {
    return null
  }
}

/** Every import of this workspace that can still be resumed, the latest first. */
export function listUnfinishedImports(workspaceId: string, now = Date.now()): SavedImport[] {
  const found: SavedImport[] = []
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (!key || !key.startsWith(`${PREFIX}${workspaceId}_`)) continue
      const saved = parse(localStorage.getItem(key), now)
      // The key must be this workspace's own: "ws_1" also starts with the prefix of "ws".
      if (saved && key === importProgressKey(workspaceId, saved.fileName)) found.push(saved)
    }
  } catch {
    return []
  }
  return found.sort((a, b) => b.timestamp - a.timestamp)
}

export function saveImport(workspaceId: string, saved: SavedImport): void {
  try {
    localStorage.setItem(importProgressKey(workspaceId, saved.fileName), JSON.stringify(saved))
  } catch {
    // A full or blocked storage loses the resume point, not the import.
  }
}

export function forgetImport(workspaceId: string, fileName: string): void {
  try {
    localStorage.removeItem(importProgressKey(workspaceId, fileName))
  } catch {
    // Nothing to forget when the storage cannot be read.
  }
}
