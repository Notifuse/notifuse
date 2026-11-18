export type BulkOperation = 'unsubscribe' | 'delete'
export type BulkOutcome = 'done' | 'skipped' | 'failed'

export interface BulkResult {
  email: string
  outcome: BulkOutcome
  detail?: string
}

/** Where a bulk update stopped, kept in this browser so the same file can pick up from there. */
export interface SavedBulkRun {
  fileName: string
  operation: BulkOperation
  listId?: string
  /** The column the addresses were read from; absent in entries written by 43.x. */
  emailColumn?: string
  /** Index, in the file's unique addresses, of the next one to process. */
  currentIndex: number
  totalEmails: number
  /** One per address before currentIndex, in file order. */
  results: BulkResult[]
  timestamp: number
}

/**
 * What is written: the addresses already done are not, since the file gives them back, only the
 * skipped and failed ones with their index. A run of a large file then fits in the storage quota.
 */
interface StoredBulkRun extends Omit<SavedBulkRun, 'results'> {
  exceptions?: Array<[number, Exclude<BulkOutcome, 'done'>, string?]>
  /** 43.x and earlier wrote every result. */
  results?: LegacyResult[]
}

const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000

/** The key of 43.x and earlier, kept so that a run saved before this version is offered. */
export function bulkProgressKey(workspaceId: string, fileName: string, operation: BulkOperation): string {
  return `bulk_update_progress_${workspaceId}_${fileName}_${operation}`
}

interface LegacyResult {
  email: string
  success?: boolean
  error?: string
  outcome?: BulkOutcome
  detail?: string
}

/** 43.x saved `{success, error}`: a success carrying an error was a skip. */
function toResult(result: LegacyResult): BulkResult {
  if (result.outcome) return { email: result.email, outcome: result.outcome, detail: result.detail }
  if (result.success && result.error) return { email: result.email, outcome: 'skipped', detail: result.error }
  if (result.success) return { email: result.email, outcome: 'done' }
  return { email: result.email, outcome: 'failed', detail: result.error }
}

export interface BulkRunMatch {
  fileName: string
  operation: BulkOperation
  listId?: string
  emailColumn: string
  /** The file's addresses, in order: they rebuild the results of the addresses done. */
  emails: string[]
}

/** The saved run of this file, action, list and column, if it can still be resumed. */
export function readBulkRun(workspaceId: string, match: BulkRunMatch, now = Date.now()): SavedBulkRun | null {
  try {
    const raw = localStorage.getItem(bulkProgressKey(workspaceId, match.fileName, match.operation))
    if (!raw) return null
    const saved = JSON.parse(raw) as StoredBulkRun
    if (
      !saved ||
      saved.fileName !== match.fileName ||
      saved.operation !== match.operation ||
      typeof saved.currentIndex !== 'number' ||
      typeof saved.totalEmails !== 'number' ||
      !(Array.isArray(saved.results) || Array.isArray(saved.exceptions)) ||
      typeof saved.timestamp !== 'number'
    ) {
      return null
    }
    if (now - saved.timestamp >= MAX_AGE_MS) return null
    if (match.operation === 'unsubscribe' && saved.listId !== match.listId) return null
    if (saved.emailColumn !== undefined && saved.emailColumn !== match.emailColumn) return null
    if (saved.currentIndex <= 0 || saved.currentIndex >= saved.totalEmails) return null
    const { exceptions, results, ...rest } = saved
    if (results) return { ...rest, results: results.map(toResult) }
    const notDone = new Map((exceptions ?? []).map(([index, outcome, detail]) => [index, { outcome, detail }]))
    return {
      ...rest,
      results: Array.from({ length: saved.currentIndex }, (_, index) => {
        const email = match.emails[index] ?? ''
        const other = notDone.get(index)
        return other ? { email, outcome: other.outcome, detail: other.detail } : { email, outcome: 'done' as const }
      })
    }
  } catch {
    return null
  }
}

export function saveBulkRun(workspaceId: string, saved: SavedBulkRun): void {
  const { results, ...rest } = saved
  const stored: StoredBulkRun = {
    ...rest,
    exceptions: results.flatMap((result, index) =>
      result.outcome === 'done' ? [] : [[index, result.outcome, result.detail] as [number, 'skipped' | 'failed', string?]]
    )
  }
  try {
    localStorage.setItem(bulkProgressKey(workspaceId, saved.fileName, saved.operation), JSON.stringify(stored))
  } catch {
    // A full or blocked storage loses the resume point, not the update.
  }
}

export function forgetBulkRun(workspaceId: string, fileName: string, operation: BulkOperation): void {
  try {
    localStorage.removeItem(bulkProgressKey(workspaceId, fileName, operation))
  } catch {
    // Nothing to forget when the storage cannot be read.
  }
}
