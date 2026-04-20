import { useCallback, useEffect, useRef, useState } from 'react'

export interface BulkActionResult {
  email: string
  success: boolean
  skipped?: boolean
  error?: string
}

export type BulkActionMode = 'iterative' | 'batch'

export interface BulkActionProgress {
  mode: BulkActionMode
  total: number
  processed: number
  succeeded: number
  skipped: number
  failed: number
  running: boolean
  paused: boolean
  cancelled: boolean
  results: BulkActionResult[]
}

export interface BulkActionRunOptions {
  delayMs?: number
}

export interface UseBulkContactActionReturn {
  progress: BulkActionProgress | null
  running: boolean
  run: (
    emails: string[],
    action: (email: string) => Promise<void>,
    opts?: BulkActionRunOptions
  ) => Promise<BulkActionResult[]>
  runBatch: (
    emails: string[],
    action: (emails: string[]) => Promise<BulkActionResult[]>
  ) => Promise<BulkActionResult[]>
  pause: () => void
  resume: () => void
  cancel: () => void
  reset: () => void
}

const DEFAULT_DELAY_MS = 100

/**
 * Throw from an action callback to mark the current email as "skipped"
 * (a benign no-op, counted as success but tagged yellow in the results list).
 */
export class SkippedAction extends Error {
  constructor(reason: string) {
    super(reason)
    this.name = 'SkippedAction'
  }
}

export function useBulkContactAction(): UseBulkContactActionReturn {
  const [progress, setProgress] = useState<BulkActionProgress | null>(null)
  const pausedRef = useRef(false)
  const cancelledRef = useRef(false)
  const busyRef = useRef(false)
  const mountedRef = useRef(true)

  useEffect(() => {
    // Restore on mount (handles StrictMode's simulated unmount+remount cycle,
    // which would otherwise leave mountedRef stuck at false from the prior
    // cleanup and silently no-op every subsequent setProgress call).
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      // Ensure any live loop exits when the owning component unmounts.
      cancelledRef.current = true
    }
  }, [])

  const safeSetProgress = useCallback(
    (updater: BulkActionProgress | null | ((prev: BulkActionProgress | null) => BulkActionProgress | null)) => {
      if (!mountedRef.current) return
      setProgress(updater)
    },
    []
  )

  const pause = useCallback(() => {
    pausedRef.current = true
    safeSetProgress((prev) => (prev ? { ...prev, paused: true } : prev))
  }, [safeSetProgress])

  const resume = useCallback(() => {
    pausedRef.current = false
    safeSetProgress((prev) => (prev ? { ...prev, paused: false } : prev))
  }, [safeSetProgress])

  const cancel = useCallback(() => {
    cancelledRef.current = true
    pausedRef.current = false
    safeSetProgress((prev) =>
      prev ? { ...prev, paused: false, cancelled: true } : prev
    )
  }, [safeSetProgress])

  const reset = useCallback(() => {
    if (busyRef.current) return
    pausedRef.current = false
    cancelledRef.current = false
    safeSetProgress(null)
  }, [safeSetProgress])

  const run = useCallback(
    async (
      emails: string[],
      action: (email: string) => Promise<void>,
      opts: BulkActionRunOptions = {}
    ): Promise<BulkActionResult[]> => {
      if (busyRef.current) {
        throw new Error('A bulk action is already running')
      }
      busyRef.current = true
      const delayMs = opts.delayMs ?? DEFAULT_DELAY_MS
      pausedRef.current = false
      cancelledRef.current = false

      const results: BulkActionResult[] = []

      try {
        safeSetProgress({
          mode: 'iterative',
          total: emails.length,
          processed: 0,
          succeeded: 0,
          skipped: 0,
          failed: 0,
          running: true,
          paused: false,
          cancelled: false,
          results: []
        })

        for (let i = 0; i < emails.length; i++) {
          if (cancelledRef.current || !mountedRef.current) break

          while (pausedRef.current && !cancelledRef.current) {
            await new Promise((resolve) => setTimeout(resolve, 100))
          }
          if (cancelledRef.current || !mountedRef.current) break

          const email = emails[i]
          const result: BulkActionResult = { email, success: false }

          try {
            await action(email)
            result.success = true
          } catch (err: unknown) {
            if (err instanceof SkippedAction) {
              result.success = true
              result.skipped = true
              result.error = err.message
            } else {
              result.error = err instanceof Error ? err.message : 'Unknown error'
            }
          }

          results.push(result)

          safeSetProgress((prev) =>
            prev
              ? {
                  ...prev,
                  processed: i + 1,
                  succeeded: prev.succeeded + (result.success && !result.skipped ? 1 : 0),
                  skipped: prev.skipped + (result.skipped ? 1 : 0),
                  failed: prev.failed + (result.success ? 0 : 1),
                  results: [...prev.results, result]
                }
              : prev
          )

          if (
            mountedRef.current &&
            i < emails.length - 1 &&
            !cancelledRef.current
          ) {
            // Interruptible sleep: exit early on cancel or unmount.
            // Use remaining time so we don't overshoot the deadline.
            await new Promise<void>((resolve) => {
              const start = Date.now()
              const tick = () => {
                const elapsed = Date.now() - start
                if (
                  cancelledRef.current ||
                  !mountedRef.current ||
                  elapsed >= delayMs
                ) {
                  resolve()
                } else {
                  const remaining = delayMs - elapsed
                  setTimeout(tick, Math.max(0, Math.min(50, remaining)))
                }
              }
              tick()
            })
          }
        }

        return results
      } finally {
        busyRef.current = false
        safeSetProgress((prev) =>
          prev ? { ...prev, running: false, paused: false } : prev
        )
      }
    },
    [safeSetProgress]
  )

  const runBatch = useCallback(
    async (
      emails: string[],
      action: (emails: string[]) => Promise<BulkActionResult[]>
    ): Promise<BulkActionResult[]> => {
      if (busyRef.current) {
        throw new Error('A bulk action is already running')
      }
      busyRef.current = true
      pausedRef.current = false
      cancelledRef.current = false

      let results: BulkActionResult[] = []

      try {
        safeSetProgress({
          mode: 'batch',
          total: emails.length,
          processed: 0,
          succeeded: 0,
          skipped: 0,
          failed: 0,
          running: true,
          paused: false,
          cancelled: false,
          results: []
        })

        try {
          results = await action(emails)
        } catch (err: unknown) {
          const errMsg = err instanceof Error ? err.message : 'Unknown error'
          results = emails.map((email) => ({ email, success: false, error: errMsg }))
        }

        safeSetProgress({
          mode: 'batch',
          total: emails.length,
          processed: emails.length,
          succeeded: results.filter((r) => r.success && !r.skipped).length,
          skipped: results.filter((r) => r.skipped).length,
          // Exclude skipped from failed to preserve the invariant
          // `succeeded + skipped + failed === processed`.
          failed: results.filter((r) => !r.success && !r.skipped).length,
          running: false,
          paused: false,
          cancelled: false,
          results
        })
        return results
      } finally {
        busyRef.current = false
      }
    },
    [safeSetProgress]
  )

  return {
    progress,
    running: progress?.running ?? false,
    run,
    runBatch,
    pause,
    resume,
    cancel,
    reset
  }
}
