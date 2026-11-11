import React, { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type Announcements,
  type DragStartEvent,
  type Modifier
} from '@dnd-kit/core'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock } from '../types'
import type { CanvasHandle } from '../panels/EditPanel'
import { CanvasDropIndicator } from '../panels/CanvasDropIndicator'
import type { CanvasDropResult } from '../utils/canvasDrop'
import type { InsertPlace } from '../utils/insertTarget'
import {
  LIBRARY_DRAG_TYPE,
  typeLabelOf,
  type LibraryDragData,
  type LibraryItem
} from '../utils/library'
import { autoScrollCanvas, locateOnCanvas } from './canvasHit'

interface LibraryDndProps {
  tree: EmailBlock
  // This editor's canvas: another editor may be mounted, hidden, at the same time
  canvasRef: React.MutableRefObject<CanvasHandle | null>
  onDrop: (item: LibraryItem, place: InsertPlace) => void
  onDraggingChange: (dragging: boolean) => void
  children: React.ReactNode
}

// The ghost sits below right of the pointer, so the line under the pointer stays visible
const besidePointer: Modifier = ({ transform, activatorEvent, activeNodeRect }) => {
  if (!activatorEvent || !activeNodeRect || !('clientX' in activatorEvent)) return transform
  const start = activatorEvent as PointerEvent
  return {
    ...transform,
    x: transform.x + start.clientX - activeNodeRect.left + 14,
    y: transform.y + start.clientY - activeNodeRect.top + 14
  }
}

const sameResult = (a: CanvasDropResult | null, b: CanvasDropResult | null) =>
  JSON.stringify(a) === JSON.stringify(b)

// Dragging a block from the library onto the canvas. dnd-kit's pointer sensor drives the gesture, with no native
// drag-and-drop (an iframe would swallow it and the text editor would insert text on drop). The place is computed
// from the pointer on every animation frame, so it follows the canvas while it scrolls, and once more at release.
export const LibraryDnd: React.FC<LibraryDndProps> = ({
  tree,
  canvasRef,
  onDrop,
  onDraggingChange,
  children
}) => {
  const { t } = useLingui()
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const [active, setActive] = useState<LibraryItem | null>(null)
  const [result, setResult] = useState<CanvasDropResult | null>(null)
  const [shownPointer, setShownPointer] = useState({ x: 0, y: 0 })
  const pointer = useRef({ x: 0, y: 0 })
  const frame = useRef(0)
  // The latest tree and handlers, for the animation loop and the drop
  const latest = useRef({ tree, onDrop, onDraggingChange })
  latest.current = { tree, onDrop, onDraggingChange }

  const locate = useCallback(
    (item: LibraryItem) =>
      locateOnCanvas(
        canvasRef.current,
        latest.current.tree,
        item.block,
        pointer.current.x,
        pointer.current.y
      ),
    [canvasRef]
  )

  const onPointerMove = useCallback((event: PointerEvent) => {
    pointer.current = { x: event.clientX, y: event.clientY }
  }, [])

  const stop = useCallback(() => {
    cancelAnimationFrame(frame.current)
    window.removeEventListener('pointermove', onPointerMove, true)
    latest.current.onDraggingChange(false)
    setResult(null)
    setActive(null)
  }, [onPointerMove])

  useEffect(() => stop, [stop])

  const handleStart = (event: DragStartEvent) => {
    const data = event.active.data.current as LibraryDragData | undefined
    if (data?.type !== LIBRARY_DRAG_TYPE) return
    const item = data.item
    const start = event.activatorEvent as PointerEvent
    pointer.current = { x: start.clientX, y: start.clientY }
    window.addEventListener('pointermove', onPointerMove, true)
    latest.current.onDraggingChange(true)
    setActive(item)
    const loop = () => {
      autoScrollCanvas(canvasRef.current, pointer.current.x, pointer.current.y)
      const next = locate(item)
      setResult((prev) => (sameResult(prev, next) ? prev : next))
      setShownPointer((prev) =>
        prev.x === pointer.current.x && prev.y === pointer.current.y ? prev : { ...pointer.current }
      )
      frame.current = requestAnimationFrame(loop)
    }
    frame.current = requestAnimationFrame(loop)
  }

  const handleEnd = () => {
    const item = active
    // Computed again at the release point: the canvas may have scrolled since the last frame
    const final = item ? locate(item) : null
    stop()
    if (item && final?.drop) {
      latest.current.onDrop(item, { parentId: final.drop.parentId, index: final.drop.index })
    }
  }

  // dnd-kit's announcements for screen readers, translated
  const nameOf = (dragged: { data: { current?: unknown } }) =>
    (dragged.data.current as LibraryDragData | undefined)?.item.name ?? ''
  const announcements: Announcements = {
    onDragStart: ({ active: dragged }) => {
      const name = nameOf(dragged)
      return t`Picked up ${name}.`
    },
    onDragOver: ({ active: dragged }) => {
      const name = nameOf(dragged)
      return t`${name} is being dragged over the email.`
    },
    onDragEnd: ({ active: dragged }) => {
      const name = nameOf(dragged)
      return t`${name} was dropped.`
    },
    onDragCancel: ({ active: dragged }) => {
      const name = nameOf(dragged)
      return t`Dragging ${name} was cancelled.`
    }
  }

  return (
    <DndContext
      sensors={sensors}
      autoScroll={false}
      onDragStart={handleStart}
      onDragEnd={handleEnd}
      onDragCancel={stop}
      accessibility={{
        announcements,
        screenReaderInstructions: {
          draggable: t`To insert a block without dragging it, use its Insert button. Press Escape to cancel a drag.`
        }
      }}
    >
      {children}
      {createPortal(
        <DragOverlay
          dropAnimation={null}
          zIndex={3000}
          modifiers={[besidePointer]}
          style={{ pointerEvents: 'none' }}
        >
          {active ? (
            <div
              data-testid="library-drag-ghost"
              className="rounded border border-[#7763F1] bg-white px-2 py-1.5 text-xs shadow-lg"
              style={{ width: 160, pointerEvents: 'none', opacity: 0.95 }}
            >
              <div className="truncate font-medium text-gray-800">{active.name}</div>
              <div className="text-[10px] text-gray-500">{typeLabelOf(active.block)}</div>
            </div>
          ) : null}
        </DragOverlay>,
        document.body
      )}
      <CanvasDropIndicator result={active ? result : null} pointer={shownPointer} />
    </DndContext>
  )
}
