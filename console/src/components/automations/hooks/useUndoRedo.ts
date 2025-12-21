import { useState, useCallback, useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { AutomationNodeData } from '../utils/flowConverter'

const MAX_HISTORY_SIZE = 200

export interface HistoryEntry {
  nodes: Node<AutomationNodeData>[]
  edges: Edge[]
}

export interface UseUndoRedoReturn {
  canUndo: boolean
  canRedo: boolean
  undo: () => HistoryEntry | null
  redo: () => HistoryEntry | null
  push: (entry: HistoryEntry) => void
  clear: () => void
}

export function useUndoRedo(): UseUndoRedoReturn {
  // History stack - past states
  const [past, setPast] = useState<HistoryEntry[]>([])
  // Future stack - states we've undone (for redo)
  const [future, setFuture] = useState<HistoryEntry[]>([])

  const canUndo = past.length > 0
  const canRedo = future.length > 0

  // Push current state to history (call BEFORE making changes)
  const push = useCallback((entry: HistoryEntry) => {
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

    const newFuture = [...future]
    const nextState = newFuture.pop()!

    setFuture(newFuture)

    return nextState
  }, [future])

  // Push current state to future (used when undoing)
  const pushToFuture = useCallback((entry: HistoryEntry) => {
    const clonedEntry: HistoryEntry = {
      nodes: structuredClone(entry.nodes),
      edges: structuredClone(entry.edges)
    }
    setFuture(prev => [...prev, clonedEntry])
  }, [])

  // Clear all history
  const clear = useCallback(() => {
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
  }), [canUndo, canRedo, undo, redo, push, clear, pushToFuture]) as UseUndoRedoReturn & { _pushToFuture: (entry: HistoryEntry) => void }
}
