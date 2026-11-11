import type { EmailBlock } from '../types'
import type { CanvasHandle } from '../panels/EditPanel'
import { resolveCanvasDrop, type Box, type CanvasDropResult } from '../utils/canvasDrop'

// The edge band of the canvas that scrolls it while a block is dragged, and the most it scrolls in a frame
export const AUTO_SCROLL_BAND = 48
export const AUTO_SCROLL_MAX = 18

// The part of an element on screen: a drawer can clip the canvas at the window's edges
const visibleBox = (element: HTMLElement): Box => {
  const r = element.getBoundingClientRect()
  return {
    left: Math.max(r.left, 0),
    right: Math.min(r.right, window.innerWidth),
    top: Math.max(r.top, 0),
    bottom: Math.min(r.bottom, window.innerHeight)
  }
}

const findBody = (tree: EmailBlock): EmailBlock | null => {
  if (tree.type === 'mj-body') return tree
  for (const child of (tree.children as EmailBlock[] | undefined) ?? []) {
    const found = findBody(child)
    if (found) return found
  }
  return null
}

const viewportOf = (canvas: CanvasHandle) => canvas.viewport() ?? canvas.root()

// Where block would land on this editor's canvas, with the pointer at x, y. Null when the pointer is not over the
// canvas. Only elements inside the canvas count: the library's thumbnails render blocks too.
export function locateOnCanvas(
  canvas: CanvasHandle | null,
  tree: EmailBlock,
  block: Pick<EmailBlock, 'type' | 'children'>,
  x: number,
  y: number
): CanvasDropResult | null {
  const root = canvas?.root()
  const viewport = canvas ? viewportOf(canvas) : null
  if (!root || !viewport) return null
  const area = visibleBox(viewport)
  if (x < area.left || x > area.right || y < area.top || y > area.bottom) return null

  const hits =
    typeof document.elementsFromPoint === 'function' ? document.elementsFromPoint(x, y) : []
  const hit = hits.find((element) => root.contains(element))
  const hovered = hit?.closest('[data-block-id]')
  const hoveredId = hovered && root.contains(hovered) ? hovered.getAttribute('data-block-id') : null

  const bodyId = findBody(tree)?.id
  const rectOf = (id: string): Box | null => {
    if (id === bodyId) return root.getBoundingClientRect()
    const element = root.querySelector(`[data-block-id=${JSON.stringify(id)}]`)
    return element ? element.getBoundingClientRect() : null
  }
  return resolveCanvasDrop({ tree, block, hoveredId, x, y, rectOf })
}

// Scrolls the canvas while the pointer is in the band along its top or bottom edge, faster deeper into the band.
// Returns how far it scrolled.
export function autoScrollCanvas(canvas: CanvasHandle | null, x: number, y: number): number {
  const viewport = canvas?.viewport()
  if (!viewport) return 0
  const area = visibleBox(viewport)
  if (x < area.left || x > area.right) return 0
  const speed = (depth: number) =>
    Math.ceil((Math.min(Math.max(depth, 0), AUTO_SCROLL_BAND) / AUTO_SCROLL_BAND) * AUTO_SCROLL_MAX)
  let delta = 0
  if (y < area.top + AUTO_SCROLL_BAND && y > area.top - AUTO_SCROLL_BAND) {
    delta = -speed(area.top + AUTO_SCROLL_BAND - y)
  } else if (y > area.bottom - AUTO_SCROLL_BAND && y < area.bottom + AUTO_SCROLL_BAND) {
    delta = speed(y - (area.bottom - AUTO_SCROLL_BAND))
  }
  if (delta) viewport.scrollTop += delta
  return delta
}
