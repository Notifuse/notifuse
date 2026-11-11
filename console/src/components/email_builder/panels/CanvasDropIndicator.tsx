import React from 'react'
import { createPortal } from 'react-dom'
import { useLingui } from '@lingui/react/macro'
import type { CanvasDropResult } from '../utils/canvasDrop'

const ACCENT = '#7763F1'

interface CanvasDropIndicatorProps {
  // The place shown for the dragged block, or why it cannot go there; nothing when null
  result: CanvasDropResult | null
  // The pointer, where a refusal shows
  pointer: { x: number; y: number }
}

// Where a block dragged from the library would land on the canvas: a line, or a box for an empty container. Over a
// place that refuses the block, no line, and the reason by the pointer.
export const CanvasDropIndicator: React.FC<CanvasDropIndicatorProps> = ({ result, pointer }) => {
  const { t } = useLingui()
  if (!result) return null

  if (!result.drop) {
    if (!result.refusal) return null
    let reason = t`Not allowed here`
    if (result.refusal === 'liquid') reason = t`Not allowed inside a conditional block`
    else if (result.reason === 'column') reason = t`Goes inside a column`
    else if (result.reason === 'section') reason = t`Goes inside a section`
    else if (result.reason === 'social') reason = t`Goes inside a social block`
    return createPortal(
      <div
        data-testid="canvas-drop-refusal"
        role="status"
        className="rounded bg-red-500 px-2 py-0.5 text-[11px] font-medium text-white shadow"
        style={{
          position: 'fixed',
          left: pointer.x + 14,
          top: pointer.y - 28,
          pointerEvents: 'none',
          zIndex: 3001,
          whiteSpace: 'nowrap'
        }}
      >
        {reason}
      </div>,
      document.body
    )
  }

  const { parentId, index, indicator } = result.drop
  const common = {
    'data-testid': 'canvas-drop-indicator',
    'data-parent-id': parentId,
    'data-index': index,
    'data-kind': indicator.kind,
    'aria-hidden': true
  }

  if (indicator.kind === 'box') {
    const { box } = indicator
    return createPortal(
      <div
        {...common}
        style={{
          position: 'fixed',
          left: box.left,
          top: box.top,
          width: box.right - box.left,
          height: box.bottom - box.top,
          border: `2px dashed ${ACCENT}`,
          background: 'rgba(119, 99, 241, 0.08)',
          boxSizing: 'border-box',
          pointerEvents: 'none',
          zIndex: 2999
        }}
      />,
      document.body
    )
  }

  const horizontal = indicator.y1 === indicator.y2
  const left = Math.min(indicator.x1, indicator.x2) - (horizontal ? 0 : 1.5)
  const top = Math.min(indicator.y1, indicator.y2) - (horizontal ? 1.5 : 0)
  return createPortal(
    <div
      {...common}
      style={{
        position: 'fixed',
        left,
        top,
        width: horizontal ? Math.abs(indicator.x2 - indicator.x1) : 3,
        height: horizontal ? 3 : Math.abs(indicator.y2 - indicator.y1),
        background: ACCENT,
        borderRadius: 2,
        pointerEvents: 'none',
        zIndex: 2999
      }}
    >
      <span
        className="rounded px-1.5 py-px text-[10px] font-medium text-white"
        style={{
          position: 'absolute',
          background: ACCENT,
          whiteSpace: 'nowrap',
          ...(horizontal ? { left: '50%', top: -9, transform: 'translateX(-50%)' } : { left: 6, top: 4 })
        }}
      >
        {t`Insert here`}
      </span>
    </div>,
    document.body
  )
}
