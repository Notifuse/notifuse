import type { EmailBlock } from '../types'
import { liquidPlacementError } from '../displayCondition'
import { nestingReason, takesFromLibrary, type NestingReason } from './libraryPlacement'

// Where a block dragged from the library lands on the canvas, from the block under the pointer. Pure: the screen boxes
// of the rendered blocks come from rectOf.

export interface Box {
  left: number
  top: number
  right: number
  bottom: number
}

export type Indicator =
  { kind: 'line'; x1: number; y1: number; x2: number; y2: number } | { kind: 'box'; box: Box }

export interface CanvasDrop {
  parentId: string
  index: number
  indicator: Indicator
}

export type CanvasDropResult =
  | { drop: CanvasDrop; refusal?: undefined }
  | { drop: null; refusal: 'nesting'; reason: NestingReason }
  | { drop: null; refusal: 'liquid' }
  // Nothing to show: the hovered block is not in this email's body, or its boxes are not there
  | { drop: null; refusal?: undefined }

export interface CanvasDropInput {
  tree: EmailBlock
  block: Pick<EmailBlock, 'type' | 'children'>
  // The deepest rendered block under the pointer; null over the canvas outside the email's blocks
  hoveredId: string | null
  x: number
  y: number
  // Screen box of a rendered block; for the body, the box of the whole email
  rectOf: (id: string) => Box | null
  // Within this distance of the edge of a container that takes the block, the drop goes beside the container when
  // its parent takes the block too
  edge?: number
}

type Axis = 'x' | 'y'

const childrenOf = (block: EmailBlock) => (block.children as EmailBlock[] | undefined) ?? []

// Columns and groups sit side by side in a section or a group, and icons in a social block unless it is vertical;
// everything else stacks
const axisOf = (container: EmailBlock): Axis => {
  if (container.type === 'mj-section' || container.type === 'mj-group') return 'x'
  if (container.type === 'mj-social') {
    return (container.attributes as Record<string, unknown> | undefined)?.mode === 'vertical'
      ? 'y'
      : 'x'
  }
  return 'y'
}

// An attribute of a block, else its default from the head's mj-attributes
const attributeOf = (tree: EmailBlock, node: EmailBlock, name: string): unknown => {
  const own = (node.attributes as Record<string, unknown> | undefined)?.[name]
  if (own !== undefined && own !== '') return own
  const head = childrenOf(tree).find((child) => child.type === 'mj-head')
  const defaults = head && childrenOf(head).find((child) => child.type === 'mj-attributes')
  const forType = defaults && childrenOf(defaults).find((child) => child.type === node.type)
  return (forType?.attributes as Record<string, unknown> | undefined)?.[name]
}

const pathTo = (tree: EmailBlock, id: string): EmailBlock[] | null => {
  if (tree.id === id) return [tree]
  for (const child of childrenOf(tree)) {
    const found = pathTo(child, id)
    if (found) return [tree, ...found]
  }
  return null
}

const findBody = (tree: EmailBlock): EmailBlock | null => {
  if (tree.type === 'mj-body') return tree
  for (const child of childrenOf(tree)) {
    const found = findBody(child)
    if (found) return found
  }
  return null
}

const start = (b: Box, axis: Axis) => (axis === 'x' ? b.left : b.top)
const end = (b: Box, axis: Axis) => (axis === 'x' ? b.right : b.bottom)
const center = (b: Box, axis: Axis) => (start(b, axis) + end(b, axis)) / 2

export function resolveCanvasDrop(input: CanvasDropInput): CanvasDropResult {
  const { tree, block, x, y, rectOf } = input
  const edge = input.edge ?? 10
  const body = findBody(tree)
  if (!body) return { drop: null }
  const pos = (axis: Axis) => (axis === 'x' ? x : y)
  const takes = (container: EmailBlock) => takesFromLibrary(container, block)

  // A right-to-left section or group draws its first child on the right. The boxes tell when there are two
  // children or more; the direction attribute otherwise.
  const reversed = (container: EmailBlock) => {
    if (axisOf(container) !== 'x') return false
    const boxes = childrenOf(container)
      .map((child) => rectOf(child.id))
      .filter((r): r is Box => !!r)
    if (boxes.length > 1) return boxes[0].left > boxes[boxes.length - 1].left
    return (
      (container.type === 'mj-section' || container.type === 'mj-group') &&
      attributeOf(tree, container, 'direction') === 'rtl'
    )
  }
  // Whether the pointer is on the side of box r that comes first in the container's order
  const beforeHalf = (r: Box, container: EmailBlock) => {
    const axis = axisOf(container)
    return reversed(container) ? pos(axis) > center(r, axis) : pos(axis) < center(r, axis)
  }

  // The line or box shown for slot `index` of container
  const indicatorFor = (container: EmailBlock, index: number): Indicator | null => {
    const axis = axisOf(container)
    const boxes = childrenOf(container)
      .map((child) => rectOf(child.id))
      .filter((r): r is Box => !!r)
    const own = rectOf(container.id)
    if (!boxes.length) return own ? { kind: 'box', box: own } : null
    const before = index > 0 ? boxes[Math.min(index, boxes.length) - 1] : null
    const after = index < boxes.length ? boxes[index] : null
    // The edge of a box that faces the next slot, and the one that faces the previous slot
    const flip = reversed(container)
    const trailing = (r: Box) => (flip ? start(r, axis) : end(r, axis))
    const leading = (r: Box) => (flip ? end(r, axis) : start(r, axis))
    const at =
      before && after
        ? (trailing(before) + leading(after)) / 2
        : before
          ? trailing(before)
          : leading(after as Box)
    const span = [before, after].filter((r): r is Box => !!r)
    if (axis === 'y') {
      return {
        kind: 'line',
        x1: Math.min(...span.map((r) => r.left)),
        y1: at,
        x2: Math.max(...span.map((r) => r.right)),
        y2: at
      }
    }
    // Side by side: the line runs the container's full height, not only its neighbours'
    const y1 = own ? own.top : Math.min(...span.map((r) => r.top))
    const y2 = own ? own.bottom : Math.max(...span.map((r) => r.bottom))
    return { kind: 'line', x1: at, y1, x2: at, y2 }
  }

  // The level that takes the block decides; a Liquid refusal there is the answer, never a level further up
  const place = (container: EmailBlock, index: number): CanvasDropResult => {
    if (liquidPlacementError(tree, container.id, block)) return { drop: null, refusal: 'liquid' }
    const indicator = indicatorFor(container, index)
    return indicator ? { drop: { parentId: container.id, index, indicator } } : { drop: null }
  }

  // Inside container, at the slot nearest the pointer
  const inside = (container: EmailBlock) => {
    const children = childrenOf(container)
    let index = children.length
    for (let i = 0; i < children.length; i++) {
      const r = rectOf(children[i].id)
      if (r && beforeHalf(r, container)) {
        index = i
        break
      }
    }
    return place(container, index)
  }

  // Beside child in parent: before it on its first half, after it on its second
  const beside = (child: EmailBlock, parent: EmailBlock) => {
    const index = childrenOf(parent).findIndex((c) => c.id === child.id)
    const r = rectOf(child.id)
    const after = r ? !beforeHalf(r, parent) : true
    return place(parent, index + (after ? 1 : 0))
  }

  const nearEdge = (r: Box, axis: Axis) =>
    pos(axis) - start(r, axis) < edge || end(r, axis) - pos(axis) < edge

  // From the hovered block up to the body, the first level that takes the block
  const chain = input.hoveredId ? pathTo(tree, input.hoveredId) : pathTo(tree, body.id)
  if (!chain) return { drop: null }
  const bodyIndex = chain.findIndex((node) => node.id === body.id)
  // A block of the head, or of no body: nothing to drop on
  if (bodyIndex === -1) return { drop: null }

  for (let i = chain.length - 1; i >= bodyIndex; i--) {
    const node = chain[i]
    const parent = i > bodyIndex ? chain[i - 1] : null
    if (takes(node)) {
      const r = rectOf(node.id)
      if (parent && takes(parent) && r && nearEdge(r, axisOf(parent))) return beside(node, parent)
      return inside(node)
    }
    if (parent && takes(parent)) return beside(node, parent)
  }

  // No level takes it. Inside a section, the nearest column of that section that does; never outside it.
  const refused: CanvasDropResult = {
    drop: null,
    refusal: 'nesting',
    reason: nestingReason(block.type)
  }
  const section = [...chain.slice(bodyIndex)].reverse().find((node) => node.type === 'mj-section')
  if (!section) return refused

  const candidates: { container: EmailBlock; r: Box }[] = []
  const walk = (node: EmailBlock) => {
    for (const child of childrenOf(node)) {
      const r = rectOf(child.id)
      if (r && takes(child)) candidates.push({ container: child, r })
      walk(child)
    }
  }
  walk(section)
  if (!candidates.length) return refused

  const distance = (r: Box) => {
    const dx = x < r.left ? r.left - x : x > r.right ? x - r.right : 0
    const dy = y < r.top ? r.top - y : y > r.bottom ? y - r.bottom : 0
    return Math.hypot(dx, dy)
  }
  // The column under the pointer's x first: under the shorter of two columns is that column's end
  const inColumn = candidates.filter((c) => x >= c.r.left && x <= c.r.right)
  const pool = inColumn.length ? inColumn : candidates
  const nearest = pool.reduce((best, c) => (distance(c.r) < distance(best.r) ? c : best))
  return inside(nearest.container)
}
