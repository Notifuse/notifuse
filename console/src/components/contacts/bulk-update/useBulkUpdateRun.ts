import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { contactsApi } from '../../../services/api/contacts'
import { contactListApi } from '../../../services/api/contact_list'
import { forgetBulkRun, saveBulkRun, type BulkOperation, type BulkResult } from './bulkProgressStore'

/** A pause between requests, so a long file does not flood the server. */
export const BULK_REQUEST_DELAY_MS = 100
const PAUSE_POLL_MS = 100
/** Results are saved every this many addresses, and on pause, stop and unmount. */
const SAVE_EVERY = 20

/** `stopping`: Stop was asked while a request is in flight; it becomes `stopped` once it is back. */
export type BulkRunStatus = 'idle' | 'running' | 'paused' | 'stopping' | 'stopped' | 'done'

export interface BulkRunState {
  status: BulkRunStatus
  operation: BulkOperation
  total: number
  /** One per processed address, in file order. */
  results: BulkResult[]
}

export interface BulkRunInput {
  fileName: string
  operation: BulkOperation
  listId?: string
  emailColumn: string
  emails: string[]
  /** 0, or the saved index of a resumed run, whose results come in `previous`. */
  fromIndex: number
  previous: BulkResult[]
}

const IDLE: BulkRunState = { status: 'idle', operation: 'unsubscribe', total: 0, results: [] }

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

export function countOutcomes(results: BulkResult[]) {
  let done = 0
  let skipped = 0
  let failed = 0
  for (const result of results) {
    if (result.outcome === 'done') done++
    else if (result.outcome === 'skipped') skipped++
    else failed++
  }
  return { done, skipped, failed }
}

/**
 * Unsubscribes or deletes a list of addresses one request at a time. As for the CSV import, each
 * run has an id that a new start, a reset or unmounting changes, and Stop marks the run so it
 * sends nothing more while what it did stays on screen and saved.
 */
export function useBulkUpdateRun(workspaceId: string) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const [state, setState] = useState<BulkRunState>(IDLE)
  const runId = useRef(0)
  const paused = useRef(false)
  const stopRequested = useRef(0)
  // What the running loop would save, for a save from outside it (unmount).
  const pending = useRef<(() => void) | null>(null)

  useEffect(
    () => () => {
      runId.current++
      pending.current?.()
    },
    []
  )

  const processOne = useCallback(
    async (operation: BulkOperation, email: string, listId?: string): Promise<BulkResult> => {
      try {
        if (operation === 'delete') {
          await contactsApi.delete({ workspace_id: workspaceId, email })
          return { email, outcome: 'done' }
        }
        const answer = await contactListApi.updateStatus({
          workspace_id: workspaceId,
          email,
          list_id: listId ?? '',
          status: 'unsubscribed'
        })
        // The server answers 200 with found: false for a contact that is not in the list.
        if (answer?.found === false) return { email, outcome: 'skipped', detail: t`Not subscribed to this list` }
        return { email, outcome: 'done' }
      } catch (error) {
        return { email, outcome: 'failed', detail: error instanceof Error ? error.message : t`Unknown error` }
      }
    },
    [workspaceId, t]
  )

  const invalidate = useCallback(() => {
    for (const key of ['contacts', 'lists', 'list-stats', 'lists-with-counts', 'total-contacts']) {
      queryClient.invalidateQueries({ queryKey: [key, workspaceId] })
    }
  }, [queryClient, workspaceId])

  /**
   * Walks `indices` of `emails`, writing each result at its index. A start walks the rest of the
   * file; a retry walks the failed ones again.
   */
  const walk = useCallback(
    async (input: BulkRunInput, results: BulkResult[], indices: number[], resumable: boolean) => {
      const id = ++runId.current
      paused.current = false
      const alive = () => runId.current === id
      const stopping = () => stopRequested.current === id
      const live: BulkRunState = { status: 'running', operation: input.operation, total: input.emails.length, results }
      setState({ ...live, results: [...results] })

      const save = () => {
        if (!resumable) return
        const next = live.results.length
        if (next >= input.emails.length) return
        saveBulkRun(workspaceId, {
          fileName: input.fileName,
          operation: input.operation,
          listId: input.operation === 'unsubscribe' ? input.listId : undefined,
          emailColumn: input.emailColumn,
          currentIndex: next,
          totalEmails: input.emails.length,
          results: live.results,
          timestamp: Date.now()
        })
      }
      pending.current = save

      let count = 0
      for (const index of indices) {
        while (paused.current && alive() && !stopping()) await sleep(PAUSE_POLL_MS)
        if (!alive() || stopping()) break
        if (count > 0) await sleep(BULK_REQUEST_DELAY_MS)
        if (!alive() || stopping()) break

        const result = await processOne(input.operation, input.emails[index], input.listId)
        // Written in place: copying the list for each address would make a long run quadratic. The
        // state still changes, as a new object, so the screen renders the new result.
        live.results[index] = result
        count++
        if (count % SAVE_EVERY === 0 || paused.current) save()
        if (!alive()) break
        setState({ ...live, status: stopping() ? 'stopping' : paused.current ? 'paused' : 'running' })
      }

      if (pending.current === save) pending.current = null
      if (count === indices.length) {
        if (resumable) forgetBulkRun(workspaceId, input.fileName, input.operation)
        if (alive()) setState({ ...live, status: 'done' })
      } else {
        // Stopped or unmounted: what was done is kept for a resume.
        save()
        if (alive()) setState({ ...live, status: 'stopped' })
      }
      if (count > 0) invalidate()
    },
    [workspaceId, processOne, invalidate]
  )

  const lastInput = useRef<BulkRunInput | null>(null)
  /** What the last walk was, so that Resume picks up a stopped retry as a retry. */
  const lastKind = useRef<'start' | 'retry'>('start')

  const start = useCallback(
    (input: BulkRunInput) => {
      lastInput.current = input
      lastKind.current = 'start'
      const results = input.previous.slice(0, input.fromIndex)
      const indices = Array.from({ length: input.emails.length - input.fromIndex }, (_, i) => input.fromIndex + i)
      return walk(input, results, indices, true)
    },
    [walk]
  )

  /** Runs the failed addresses again; their new results replace the old ones. */
  const retryFailed = useCallback(() => {
    const input = lastInput.current
    if (!input) return Promise.resolve()
    lastKind.current = 'retry'
    const results = state.results
    const indices = results.flatMap((result, index) => (result?.outcome === 'failed' ? [index] : []))
    return walk(input, [...results], indices, false)
  }, [walk, state.results])

  /** Picks up a stopped run where it stopped; a stopped retry retries the failed addresses left. */
  const resume = useCallback(() => {
    const input = lastInput.current
    if (!input) return Promise.resolve()
    if (lastKind.current === 'retry') return retryFailed()
    return start({ ...input, fromIndex: state.results.length, previous: state.results })
  }, [start, retryFailed, state.results])

  const pause = useCallback(() => {
    paused.current = true
    setState((s) => (s.status === 'running' ? { ...s, status: 'paused' } : s))
  }, [])

  const unpause = useCallback(() => {
    paused.current = false
    setState((s) => (s.status === 'paused' ? { ...s, status: 'running' } : s))
  }, [])

  const stop = useCallback(() => {
    stopRequested.current = runId.current
    paused.current = false
    setState((s) => (s.status === 'running' || s.status === 'paused' ? { ...s, status: 'stopping' } : s))
  }, [])

  const reset = useCallback(() => {
    runId.current++
    paused.current = false
    pending.current = null
    lastInput.current = null
    setState(IDLE)
  }, [])

  return { state, start, resume, retryFailed, pause, unpause, stop, reset }
}
