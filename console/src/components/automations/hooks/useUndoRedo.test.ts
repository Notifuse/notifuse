import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useUndoRedo, type HistoryEntry } from './useUndoRedo'
import type { Node } from '@xyflow/react'
import type { AutomationNodeData } from '../utils/flowConverter'

const entry = (description: string): HistoryEntry => ({
  nodes: [
    {
      id: 'n1',
      type: 'delay',
      position: { x: 0, y: 0 },
      data: { nodeType: 'delay', config: { description }, label: 'Delay' }
    } as Node<AutomationNodeData>
  ],
  edges: []
})

const describedAs = (state: HistoryEntry | null) =>
  (state?.nodes[0].data.config as { description?: string } | undefined)?.description

describe('useUndoRedo coalescing', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('records one undo step for a run of edits sharing a key', () => {
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      // Each push snapshots the state BEFORE that keystroke, so the run's checkpoint is "".
      result.current.push(entry(''), 'n1:description')
      result.current.push(entry('W'), 'n1:description')
      result.current.push(entry('We'), 'n1:description')
      result.current.push(entry('Wel'), 'n1:description')
    })

    let undone: HistoryEntry | null = null
    act(() => {
      undone = result.current.undo()
    })

    expect(describedAs(undone)).toBe('')
    expect(result.current.canUndo).toBe(false)
  })

  it('starts a new step once the run goes quiet', () => {
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry(''), 'n1:description')
      result.current.push(entry('W'), 'n1:description')
    })
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    act(() => {
      result.current.push(entry('We'), 'n1:description')
      result.current.push(entry('Wel'), 'n1:description')
    })

    // Two checkpoints: before the first burst, and before the second.
    let first: HistoryEntry | null = null
    let second: HistoryEntry | null = null
    act(() => {
      first = result.current.undo()
    })
    act(() => {
      second = result.current.undo()
    })

    expect(describedAs(first)).toBe('We')
    expect(describedAs(second)).toBe('')
  })

  it('keeps a continuous run together however long it lasts', () => {
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry(''), 'n1:description')
    })
    for (let i = 0; i < 10; i++) {
      act(() => {
        vi.advanceTimersByTime(900)
        result.current.push(entry(`${i}`), 'n1:description')
      })
    }

    act(() => {
      result.current.undo()
    })
    expect(result.current.canUndo).toBe(false)
  })

  it('ends the run when a different field is edited', () => {
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry(''), 'n1:description')
      result.current.push(entry('W'), 'n1:description')
      result.current.push(entry('We'), 'n1:url')
    })

    expect(result.current.canUndo).toBe(true)
    act(() => {
      result.current.undo()
    })
    act(() => {
      result.current.undo()
    })
    expect(result.current.canUndo).toBe(false)
  })

  it('ends the run for a push carrying no key', () => {
    // Structural edits — adding a node, dragging, connecting — must never be swallowed by an open
    // typing run, or the flow's shape becomes unrecoverable.
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry(''), 'n1:description')
      result.current.push(entry('W'), 'n1:description')
      result.current.push(entry('We'))
      result.current.push(entry('We'), 'n1:description')
    })

    // Three checkpoints: the typing run, the structural edit that closed it, and the run after.
    act(() => {
      result.current.undo()
    })
    act(() => {
      result.current.undo()
    })
    expect(result.current.canUndo).toBe(true)
    act(() => {
      result.current.undo()
    })
    expect(result.current.canUndo).toBe(false)
  })

  it('does not swallow the first edit after an undo', () => {
    // The run is closed by the undo itself; otherwise the next keystroke would be treated as a
    // continuation and leave no checkpoint at all for the state it just restored.
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry(''), 'n1:description')
      result.current.push(entry('W'), 'n1:description')
    })
    act(() => {
      result.current.undo()
    })
    expect(result.current.canUndo).toBe(false)

    act(() => {
      result.current.push(entry(''), 'n1:description')
    })
    expect(result.current.canUndo).toBe(true)
  })

  it('still clears the redo stack when a run begins', () => {
    const { result } = renderHook(() => useUndoRedo())

    act(() => {
      result.current.push(entry('a'))
    })
    act(() => {
      result.current.undo()
    })
    act(() => {
      result.current.redo()
    })
    expect(result.current.canRedo).toBe(false)

    act(() => {
      result.current.push(entry('b'), 'n1:description')
    })
    expect(result.current.canRedo).toBe(false)
  })
})
