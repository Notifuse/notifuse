import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'

// Where a dragged block goes: index is its position in parent.children once the block has been taken out
export interface DropTarget {
  parentId: string
  index: number
}

// The space between two rows of the structure tree, as shown on screen: upper is the row above the pointer's gap,
// lower the row below it, or null after the last row
export interface DropGap {
  upperId: string
  lowerId: string | null
}

// A place the gap offers. slot counts in the parent's current children, depth from the body (0). emptyInside marks
// the inside of an empty container
export interface DropPlace {
  parentId: string
  slot: number
  depth: number
  emptyInside: boolean
}

// The dragged block: a block of the tree (id set) or a new one of the given type. depth is the level it is dragged
// from, which the pointer's sideways moves count from
export interface DragSource {
  id?: string
  type: MJMLComponentType
  depth: number
}

interface Located {
  block: EmailBlock
  // Ancestors from the root down to the block's parent
  path: EmailBlock[]
}

const locate = (tree: EmailBlock, id: string): Located | null => {
  const walk = (block: EmailBlock, path: EmailBlock[]): Located | null => {
    if (block.id === id) return { block, path }
    for (const child of block.children ?? []) {
      const found = walk(child, [...path, block])
      if (found) return found
    }
    return null
  }
  return walk(tree, [])
}

const contains = (ancestor: EmailBlock, id: string): boolean =>
  ancestor.id === id || (ancestor.children ?? []).some((child) => contains(child, id))

const isBody = (block: EmailBlock) => block.type === 'mj-body'

const indexIn = (parent: EmailBlock, id: string) =>
  (parent.children ?? []).findIndex((child) => child.id === id)

// Levels from the body, or null for a block outside it
export function depthOf(tree: EmailBlock, id: string): number | null {
  const found = locate(tree, id)
  if (!found) return null
  if (isBody(found.block)) return 0
  const body = found.path.findIndex(isBody)
  return body === -1 ? null : found.path.length - body
}

/**
 * The places a gap offers, deepest first: inside the upper block when it is a container, then after the upper
 * block and after each ancestor whose shown content ends with it, up to the body's children.
 */
export function placesInGap(tree: EmailBlock, gap: DropGap): DropPlace[] {
  const upper = locate(tree, gap.upperId)
  const upperDepth = depthOf(tree, gap.upperId)
  if (!upper || upperDepth === null) return []
  if (gap.lowerId !== null && !locate(tree, gap.lowerId)) return []
  const lowerId = gap.lowerId

  const places: DropPlace[] = []
  const block = upper.block
  if (EmailBlockClass.from(block).canHaveChildren()) {
    const children = block.children ?? []
    const lowerIndex = lowerId === null ? -1 : indexIn(block, lowerId)
    // The lower row is a child of the upper one: the container is shown open, the gap is before that child
    const slot = lowerIndex !== -1 ? lowerIndex : children.length
    places.push({ parentId: block.id, slot, depth: upperDepth + 1, emptyInside: !children.length })
  }

  let current = block
  let depth = upperDepth
  for (let i = upper.path.length - 1; i >= 0 && !isBody(current); i--) {
    if (lowerId !== null && contains(current, lowerId)) break
    const parent = upper.path[i]
    places.push({
      parentId: parent.id,
      slot: indexIn(parent, current.id) + 1,
      depth,
      emptyInside: false
    })
    current = parent
    depth--
  }
  return places
}

/**
 * The place a drop in the gap gives, or null when there is none (the line is then not shown). shift is the number
 * of indents the pointer moved right (positive) or left since the grab.
 *
 * An empty container that accepts the block takes it unless the pointer moved left. Otherwise the place nearest to
 * the pointer's depth wins, the deeper one on a tie. A place that is the block's current one gives null.
 */
export function resolveDrop(
  tree: EmailBlock,
  drag: DragSource,
  gap: DropGap,
  shift: number
): (DropTarget & { depth: number }) | null {
  const dragged = drag.id === undefined ? null : locate(tree, drag.id)
  if (drag.id !== undefined && !dragged) return null

  const kept = placesInGap(tree, gap).filter((place) => {
    if (dragged && contains(dragged.block, place.parentId)) return false
    const parent = locate(tree, place.parentId)
    return !!parent && EmailBlockClass.from(parent.block).canAcceptChild(drag.type)
  })
  if (!kept.length) return null

  const pointerDepth = drag.depth + shift
  const chosen =
    (shift >= 0 && kept.find((place) => place.emptyInside)) ||
    kept.reduce((best, place) => {
      const distance = Math.abs(place.depth - pointerDepth)
      const bestDistance = Math.abs(best.depth - pointerDepth)
      return distance < bestDistance || (distance === bestDistance && place.depth > best.depth)
        ? place
        : best
    })

  // Turn the slot, counted in the parent's current children, into the final index
  const parent = locate(tree, chosen.parentId)!.block
  const current = drag.id === undefined ? -1 : indexIn(parent, drag.id)
  const index = current !== -1 && current < chosen.slot ? chosen.slot - 1 : chosen.slot
  if (current !== -1 && current === index) return null
  return { parentId: chosen.parentId, index, depth: chosen.depth }
}
