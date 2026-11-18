import { useCallback, useEffect, useRef, useState } from 'react'
import { useLingui } from '@lingui/react/macro'
import { contactsApi, type Contact } from '../../../services/api/contacts'
import { rowToContact, type ColumnMappings } from './contactFields'
import { forgetImport, saveImport } from './importProgressStore'

export const IMPORT_BATCH_SIZE = 100
const PAUSE_POLL_MS = 100

/** `stopping`: Stop was asked while a batch is in flight; it becomes `stopped` once that batch is back. */
export type ImportRunStatus = 'idle' | 'running' | 'paused' | 'stopping' | 'stopped' | 'done'

export interface ImportFailure {
  /** Index of the data row in the file (0 = the first row under the header). */
  rowIndex: number
  /** The line in the file, header included, as a spreadsheet numbers it. */
  line: number
  email: string
  error: string
}

export interface ImportRunState {
  status: ImportRunStatus
  /** Index of the next row to send; equals totalRows once done. */
  row: number
  totalRows: number
  /** The batch being sent (1-based). */
  batch: number
  totalBatches: number
  /** Counted over this run (a resumed import counts from where it resumed). */
  created: number
  updated: number
  failed: number
  /** Rows whose email comes again later in the same batch: the server keeps the last one only. */
  merged: number
  failures: ImportFailure[]
}

export interface ImportRunInput {
  fileName: string
  headers: string[]
  rows: unknown[][]
  mappings: ColumnMappings
  listIds: string[]
  /** The row to start from: 0, or the saved row of a resumed import. */
  fromRow: number
}

const IDLE: ImportRunState = {
  status: 'idle',
  row: 0,
  totalRows: 0,
  batch: 0,
  totalBatches: 0,
  created: 0,
  updated: 0,
  failed: 0,
  merged: 0,
  failures: []
}

/** The 1-based batch that starts at `row`: the one being sent, or the last one once all are sent. */
const batchAt = (row: number, totalBatches: number) =>
  Math.max(1, Math.min(Math.floor(row / IMPORT_BATCH_SIZE) + 1, totalBatches))

/** The contact an error of contacts.import is about: "invalid contact at index 3: …" → 3. */
export function indexInError(error: string | undefined): number | undefined {
  const match = /\bat index (\d+)\b/.exec(error ?? '')
  return match ? Number(match[1]) : undefined
}

/** "invalid contact at index 3: email is required" → "email is required". */
export function withoutIndex(error: string | undefined): string {
  return (error ?? '').replace(/^.*?\bat index \d+:\s*/, '')
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

/**
 * Sends a parsed CSV to contacts.import in batches. Each run has an id: a new start, a reset and
 * unmounting change it, and the old loop then sends and shows nothing more. Stop marks the run
 * instead: it sends nothing more either, but the batch already in flight is still counted, shown
 * and saved.
 */
export function useCsvImportRun({ workspaceId, onImported }: { workspaceId: string; onImported?: () => void }) {
  const { t } = useLingui()
  const [state, setState] = useState<ImportRunState>(IDLE)
  const runId = useRef(0)
  const paused = useRef(false)
  const stopRequested = useRef(0)
  const onImportedRef = useRef(onImported)
  useEffect(() => {
    onImportedRef.current = onImported
  }, [onImported])

  // Unmounting (the drawer closed) stops the run where it is; its last batch is already saved.
  useEffect(
    () => () => {
      runId.current++
    },
    []
  )

  const start = useCallback(
    async (input: ImportRunInput) => {
      const id = ++runId.current
      paused.current = false
      const { fileName, headers, rows, mappings, listIds } = input
      const totalRows = rows.length
      const totalBatches = Math.ceil(totalRows / IMPORT_BATCH_SIZE)
      const live: ImportRunState = {
        ...IDLE,
        status: 'running',
        row: input.fromRow,
        totalRows,
        batch: batchAt(input.fromRow, totalBatches),
        totalBatches
      }
      setState({ ...live })

      const alive = () => runId.current === id
      const stopping = () => stopRequested.current === id

      while (live.row < totalRows) {
        while (paused.current && alive() && !stopping()) await sleep(PAUSE_POLL_MS)
        if (!alive() || stopping()) break

        const end = Math.min(live.row + IMPORT_BATCH_SIZE, totalRows)
        const sent: Array<{ contact: Partial<Contact>; rowIndex: number }> = []
        const failures: ImportFailure[] = []
        for (let rowIndex = live.row; rowIndex < end; rowIndex++) {
          const contact = rowToContact(rows[rowIndex], headers, mappings)
          if (contact.email) sent.push({ contact, rowIndex })
          else failures.push({ rowIndex, line: rowIndex + 2, email: '', error: t`no email address` })
        }

        // The server keeps the last row of an email repeated in the batch and answers for it only.
        live.merged += sent.length - new Set(sent.map((s) => s.contact.email)).size

        if (sent.length > 0) {
          try {
            const result = await contactsApi.batchImport({
              workspace_id: workspaceId,
              contacts: sent.map((s) => s.contact),
              subscribe_to_lists: listIds.length > 0 ? listIds : undefined
            })
            if (result.error) {
              sent.forEach((s) =>
                failures.push({ rowIndex: s.rowIndex, line: s.rowIndex + 2, email: s.contact.email ?? '', error: result.error! })
              )
            } else {
              // The server answers its validation errors first, then the upserts, an email repeated
              // in the batch only once: operations do not line up with the contacts sent. An error
              // names its contact by index in its message ("invalid contact at index 3: …").
              result.operations.forEach((operation) => {
                if (operation.action === 'create') live.created++
                else if (operation.action === 'update') live.updated++
                else {
                  const index = indexInError(operation.error)
                  const source =
                    (index !== undefined ? sent[index] : undefined) ??
                    sent.find((s) => !!operation.email && s.contact.email === operation.email)
                  failures.push({
                    rowIndex: source?.rowIndex ?? -1,
                    line: source ? source.rowIndex + 2 : 0,
                    email: operation.email || source?.contact.email || '',
                    // The index is the contact's place in the batch, not a line of the file: the Line
                    // column says where it is.
                    error: withoutIndex(operation.error) || t`Unknown error`
                  })
                }
              })
            }
          } catch (error) {
            const text = error instanceof Error ? error.message : t`Unknown error`
            sent.forEach((s) =>
              failures.push({ rowIndex: s.rowIndex, line: s.rowIndex + 2, email: s.contact.email ?? '', error: text })
            )
          }
        }

        failures.sort((a, b) => a.rowIndex - b.rowIndex)
        live.failed += failures.length
        live.failures = [...live.failures, ...failures]
        live.row = end
        live.batch = batchAt(end, totalBatches)

        if (live.row < totalRows) {
          saveImport(workspaceId, {
            fileName,
            currentRow: live.row,
            totalRows,
            currentBatch: live.batch,
            totalBatches,
            mappings,
            selectedListIds: listIds,
            timestamp: Date.now()
          })
        }
        // A run replaced or unmounted while its batch was in flight shows nothing more.
        if (!alive()) break
        setState({ ...live, status: stopping() ? 'stopped' : paused.current ? 'paused' : 'running' })
        if (stopping()) break
      }

      const imported = live.created + live.updated > 0
      if (live.row >= totalRows) {
        forgetImport(workspaceId, fileName)
        if (alive()) setState({ ...live, status: 'done' })
        onImportedRef.current?.()
      } else {
        // Stopped between two batches (while paused): nothing was in flight to wait for.
        if (alive() && stopping()) setState({ ...live, status: 'stopped' })
        if (imported) onImportedRef.current?.()
      }
    },
    [workspaceId, t]
  )

  const pause = useCallback(() => {
    paused.current = true
    setState((s) => (s.status === 'running' ? { ...s, status: 'paused' } : s))
  }, [])

  const resume = useCallback(() => {
    paused.current = false
    setState((s) => (s.status === 'paused' ? { ...s, status: 'running' } : s))
  }, [])

  const stop = useCallback(() => {
    stopRequested.current = runId.current
    paused.current = false
    // The loop turns it into `stopped` once the batch in flight is back, counted and saved: until
    // then, offering to resume would start a second run over the first.
    setState((s) => (s.status === 'running' || s.status === 'paused' ? { ...s, status: 'stopping' } : s))
  }, [])

  const reset = useCallback(() => {
    runId.current++
    paused.current = false
    setState(IDLE)
  }, [])

  return { state, start, pause, resume, stop, reset }
}
