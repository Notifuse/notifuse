import type { Viewport } from '@xyflow/react'
import { DEFAULT_NODE_HEIGHT, NODE_WIDTH } from '../nodes/outputLayout'

/** Space between the top of the canvas and the trigger once the view is anchored on it. */
export const TOP_GAP = 48

interface AnchorNode {
  position: { x: number; y: number }
  measured?: { width?: number; height?: number }
}

/**
 * The view that puts the trigger `top` px from the top of the canvas (under the banner when one
 * shows), centred across its width, at `zoom`. The editor opens on it.
 */
export function anchorOnTrigger(trigger: AnchorNode, canvasWidth: number, zoom: number, top = TOP_GAP): Viewport {
  const width = trigger.measured?.width ?? 0
  return {
    x: canvasWidth / 2 - (trigger.position.x + width / 2) * zoom,
    y: top - trigger.position.y * zoom,
    zoom
  }
}

/** Space kept between a step brought back into view and the edge of the canvas: room for the
 * delete button beside a selected step. */
export const REVEAL_MARGIN = 48

// How far to move the view along one axis so a box (`start`, `size`, in flow units) shows with
// REVEAL_MARGIN to spare; a box larger than the canvas lines up on the near edge.
function revealAlong(start: number, size: number, offset: number, zoom: number, canvas: number): number {
  const near = start * zoom + offset
  const far = near + size * zoom
  if (near < REVEAL_MARGIN) return REVEAL_MARGIN - near
  if (far > canvas - REVEAL_MARGIN) return Math.max(canvas - REVEAL_MARGIN - far, REVEAL_MARGIN - near)
  return 0
}

/**
 * How far to pan the canvas across so a step it hides (behind the step panel that just opened, or
 * past the left edge) is in view again; 0 when it already is. A step wider than the canvas lines up
 * on the left edge.
 */
export function revealDelta(step: AnchorNode, view: { x: number; zoom: number }, canvasWidth: number): number {
  return revealAlong(step.position.x, step.measured?.width || NODE_WIDTH, view.x, view.zoom, canvasWidth)
}

/** The same, up and down: a step the layout moved above or below the canvas comes back into view. */
export function revealDeltaY(step: AnchorNode, view: { y: number; zoom: number }, canvasHeight: number): number {
  return revealAlong(step.position.y, step.measured?.height || DEFAULT_NODE_HEIGHT, view.y, view.zoom, canvasHeight)
}
