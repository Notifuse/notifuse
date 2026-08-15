import { useState, useCallback, useMemo, useRef } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { AutomationNodeData } from '../utils/flowConverter'

const MAX_HISTORY_SIZE = 200

// How long a run of edits sharing a coalesce key stays a single undo step. Refreshed on every
// coalesced edit, so continuous typing is one step however long it runs, while a pause long enough
// to be a new thought starts a new one.
const COALESCE_WINDOW_MS = 1000

export interface HistoryEntry {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
}

export interface UseUndoRedoReturn {
  canUndo: boolean
  canRedo: boolean
  undo: () => HistoryEntry | null
  redo: () => HistoryEntry | null
  /**
   * Snapshot the current state as an undo point. Call BEFORE making the change.
   *
   * `coalesceKey` identifies a run of edits that should collapse into one undo step — typing in a
   * single field, say. Consecutive pushes carrying the same key inside the coalesce window keep the
   * snapshot taken before the run started and record nothing further, so undo rewinds the whole run
   * rather than one keystroke. Any push without a key (or with a different one) ends the run.
   */
  push: (entry: HistoryEntry, coalesceKey?: string) => void
  clear: () => void
  /**
   * Internal — push the current state onto the future (redo) stack. The provider calls this when
   * undoing so it can snapshot the state it is about to replace; nothing else should use it.
   */
  _pushToFuture: (entry: HistoryEntry) => void
}

export function useUndoRedo(): UseUndoRedoReturn {
  // History stack - past states
  const [past, setPast] = useState<HistoryEntry[]>([])
  // Future stack - states we've undone (for redo)
  const [future, setFuture] = useState<HistoryEntry[]>([])

  // The run of coalesced edits currently in progress, if any.
  const openRunRef = useRef<{ key: string; at: number } | null>(null)

  const canUndo = past.length > 0
  const canRedo = future.length > 0

  // Push current state to history (call BEFORE making changes)
  const push = useCallback((entry: HistoryEntry, coalesceKey?: string) => {
    if (coalesceKey) {
      const openRun = openRunRef.current
      const now = Date.now()
      openRunRef.current = { key: coalesceKey, at: now }

      // Already inside this run: the snapshot from before it started is the checkpoint to keep, and
      // returning here also skips the two state updates below — a keystroke costs no re-render and
      // no clone of the whole flow.
      if (openRun && openRun.key === coalesceKey && now - openRun.at < COALESCE_WINDOW_MS) {
        return
      }
    } else {
      openRunRef.current = null
    }

    // Deep clone the entry to avoid reference issues
    const clonedEntry: HistoryEntry = {
      nodes: structuredClone(entry.nodes),
      edges: structuredClone(entry.edges)
    }

    setPast(prev => {
      const newPast = [...prev, clonedEntry]
      // Trim to max size
      if (newPast.length > MAX_HISTORY_SIZE) {
        return newPast.slice(-MAX_HISTORY_SIZE)
      }
      return newPast
    })

    // Clear future when new action is taken
    setFuture([])
  }, [])

  // Undo - restore previous state
  const undo = useCallback((): HistoryEntry | null => {
    if (past.length === 0) return null

    // Any of these ends an open run: the next edit must start its own checkpoint.
    openRunRef.current = null

    const newPast = [...past]
    const previousState = newPast.pop()!

    setPast(newPast)

    // Note: The current state will be pushed to future by the caller
    // This allows the caller to save current state before restoring

    return previousState
  }, [past])

  // Redo - restore next state from future
  const redo = useCallback((): HistoryEntry | null => {
    if (future.length === 0) return null

    // Ends any open run: the next edit must start its own checkpoint.
    openRunRef.current = null

    const newFuture = [...future]
    const nextState = newFuture.pop()!

    setFuture(newFuture)

    return nextState
  }, [future])

  // Push current state to future (used when undoing)
  const pushToFuture = useCallback((entry: HistoryEntry) => {
    // Ends any open run: the next edit must start its own checkpoint.
    openRunRef.current = null
    const clonedEntry: HistoryEntry = {
      nodes: structuredClone(entry.nodes),
      edges: structuredClone(entry.edges)
    }
    setFuture(prev => [...prev, clonedEntry])
  }, [])

  // Clear all history
  const clear = useCallback(() => {
    // Ends any open run: the next edit must start its own checkpoint.
    openRunRef.current = null
    setPast([])
    setFuture([])
  }, [])

  return useMemo(() => ({
    canUndo,
    canRedo,
    undo,
    redo,
    push,
    clear,
    // Internal - exposed for context to handle undo properly
    _pushToFuture: pushToFuture
  }), [canUndo, canRedo, undo, redo, push, clear, pushToFuture])
}
