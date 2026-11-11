import React from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LibraryDnd } from './LibraryDnd'
import type { CanvasHandle } from '../panels/EditPanel'
import { useLibraryDraggable, type LibraryItem } from '../utils/library'
import { b, email, text } from '../utils/libraryTestTrees'

// jsdom has no PointerEvent, which dnd-kit's pointer sensor reads (isPrimary, button, clientX)
class TestPointerEvent extends MouseEvent {
  pointerId: number
  isPrimary: boolean
  pointerType: string
  constructor(type: string, init: PointerEventInit = {}) {
    super(type, { bubbles: true, cancelable: true, ...init })
    this.pointerId = init.pointerId ?? 1
    this.isPrimary = init.isPrimary ?? true
    this.pointerType = init.pointerType ?? 'mouse'
  }
}

interface Rect {
  left: number
  top: number
  right: number
  bottom: number
}

const domRect = ({ left, top, right, bottom }: Rect) =>
  ({
    left,
    top,
    right,
    bottom,
    x: left,
    y: top,
    width: right - left,
    height: bottom - top,
    toJSON: () => ({})
  }) as DOMRect

// One section with one column holding one text, laid out top to bottom: S 0-200, C 0-200, T 0-100
const tree = email([b('S', 'mj-section', [b('C', 'mj-column', [text('T')])])])
const layout: Record<string, Rect> = {
  root: { left: 0, top: 0, right: 600, bottom: 400 },
  S: { left: 0, top: 0, right: 600, bottom: 200 },
  C: { left: 0, top: 0, right: 600, bottom: 200 },
  T: { left: 0, top: 0, right: 600, bottom: 100 }
}

const item: LibraryItem = { kind: 'saved', key: 'sig', name: 'Signature', block: text('saved-t') }

const Card: React.FC = () => {
  const { setNodeRef, listeners, attributes } = useLibraryDraggable(item)
  return (
    <div ref={setNodeRef} {...listeners} {...attributes} data-testid="card">
      {item.name}
    </div>
  )
}

const buildCanvas = () => {
  const root = document.createElement('div')
  root.innerHTML =
    '<div data-block-id="S"><div data-block-id="C"><div data-block-id="T"></div></div></div>'
  document.body.appendChild(root)
  const rectOf = (element: Element) => {
    const id = element === root ? 'root' : element.getAttribute('data-block-id')
    return domRect(layout[id ?? ''] ?? { left: 0, top: 0, right: 0, bottom: 0 })
  }
  for (const element of [root, ...Array.from(root.querySelectorAll('[data-block-id]'))]) {
    element.getBoundingClientRect = () => rectOf(element)
  }
  // The deepest block whose box holds the point, as the browser would hit it
  const at = (x: number, y: number): Element[] => {
    const inside = (id: string) => {
      const r = layout[id]
      return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom
    }
    const hits: Element[] = []
    for (const id of ['T', 'C', 'S']) {
      if (inside(id)) hits.push(root.querySelector(`[data-block-id="${id}"]`)!)
    }
    if (inside('root')) hits.push(root)
    return hits
  }
  const handle: CanvasHandle = { root: () => root, viewport: () => root }
  return { root, handle, at }
}

describe('LibraryDnd', () => {
  let frames: FrameRequestCallback[] = []
  let canvas: ReturnType<typeof buildCanvas>

  const runFrames = () =>
    act(() => {
      const queued = frames
      frames = []
      queued.forEach((callback) => callback(0))
    })

  const setup = () => {
    const onDrop = vi.fn()
    const onDraggingChange = vi.fn()
    const ref = { current: canvas.handle }
    render(
      <LibraryDnd tree={tree} canvasRef={ref} onDrop={onDrop} onDraggingChange={onDraggingChange}>
        <Card />
      </LibraryDnd>
    )
    return { onDrop, onDraggingChange }
  }

  // Presses on the card and moves past the sensor's distance, which starts the drag
  const startDrag = () => {
    const card = screen.getByTestId('card')
    card.getBoundingClientRect = () => domRect({ left: 700, top: 10, right: 860, bottom: 60 })
    fireEvent.pointerDown(card, { clientX: 720, clientY: 20, button: 0, isPrimary: true })
    fireEvent.pointerMove(document, { clientX: 730, clientY: 28 })
    runFrames()
  }

  const moveTo = (x: number, y: number) => {
    fireEvent.pointerMove(document, { clientX: x, clientY: y })
    runFrames()
  }

  const indicator = () => document.querySelector('[data-testid="canvas-drop-indicator"]')

  beforeEach(() => {
    frames = []
    vi.stubGlobal('PointerEvent', TestPointerEvent)
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) =>
      frames.push(callback)
    )
    vi.stubGlobal('cancelAnimationFrame', () => {
      frames = []
    })
    canvas = buildCanvas()
    document.elementsFromPoint = (x: number, y: number) => canvas.at(x, y)
  })

  afterEach(() => {
    canvas.root.remove()
    vi.unstubAllGlobals()
    // @ts-expect-error jsdom has none; each test sets its own
    delete document.elementsFromPoint
  })

  it('shows the place under the pointer and drops the block there', () => {
    const { onDrop, onDraggingChange } = setup()
    startDrag()
    expect(onDraggingChange).toHaveBeenLastCalledWith(true)
    expect(screen.getByTestId('library-drag-ghost')).toHaveTextContent('Signature')

    moveTo(300, 80)
    expect(indicator()).toHaveAttribute('data-parent-id', 'C')
    expect(indicator()).toHaveAttribute('data-index', '1')

    fireEvent.pointerUp(document, { clientX: 300, clientY: 80 })
    expect(onDrop).toHaveBeenCalledWith(item, { parentId: 'C', index: 1 })
    expect(onDraggingChange).toHaveBeenLastCalledWith(false)
    expect(indicator()).toBeNull()
  })

  it('computes the place again at release, without waiting for a frame', () => {
    const { onDrop } = setup()
    startDrag()
    moveTo(300, 80)
    expect(indicator()).toHaveAttribute('data-index', '1')
    // The pointer moves to the top half of the text and is released before any frame runs
    fireEvent.pointerMove(document, { clientX: 300, clientY: 20 })
    fireEvent.pointerUp(document, { clientX: 300, clientY: 20 })
    expect(onDrop).toHaveBeenCalledWith(item, { parentId: 'C', index: 0 })
  })

  it('drops nothing when released outside the canvas', () => {
    const { onDrop, onDraggingChange } = setup()
    startDrag()
    moveTo(300, 80)
    moveTo(800, 80)
    expect(indicator()).toBeNull()
    fireEvent.pointerUp(document, { clientX: 800, clientY: 80 })
    expect(onDrop).not.toHaveBeenCalled()
    expect(onDraggingChange).toHaveBeenLastCalledWith(false)
  })

  it('drops nothing on a place that refuses the block, and says why', () => {
    const { onDrop } = setup()
    startDrag()
    // Below the email's only section: the body, where a text does not go
    moveTo(300, 300)
    expect(indicator()).toBeNull()
    expect(screen.getByTestId('canvas-drop-refusal')).toHaveTextContent('Goes inside a column')
    fireEvent.pointerUp(document, { clientX: 300, clientY: 300 })
    expect(onDrop).not.toHaveBeenCalled()
    expect(screen.queryByTestId('canvas-drop-refusal')).toBeNull()
  })

  it('Escape cancels the drag', () => {
    const { onDrop, onDraggingChange } = setup()
    startDrag()
    moveTo(300, 80)
    fireEvent.keyDown(document, { key: 'Escape', code: 'Escape' })
    expect(indicator()).toBeNull()
    expect(onDraggingChange).toHaveBeenLastCalledWith(false)
    fireEvent.pointerUp(document, { clientX: 300, clientY: 80 })
    expect(onDrop).not.toHaveBeenCalled()
  })

  it('a press without moving does not start a drag', () => {
    const { onDraggingChange } = setup()
    const card = screen.getByTestId('card')
    fireEvent.pointerDown(card, { clientX: 720, clientY: 20, button: 0, isPrimary: true })
    fireEvent.pointerMove(document, { clientX: 721, clientY: 21 })
    fireEvent.pointerUp(document, { clientX: 721, clientY: 21 })
    expect(onDraggingChange).not.toHaveBeenCalled()
  })
})
