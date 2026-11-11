import type { EmailBlock } from '../types'
import { liquidPlacementError } from '../displayCondition'
import { nestingReason, takesFromLibrary, type NestingReason } from './libraryPlacement'

export interface InsertPlace {
  parentId: string
  index: number
}

export type InsertRefusal = { refusal: 'nesting'; reason: NestingReason } | { refusal: 'liquid' }

export type InsertTarget = InsertPlace | InsertRefusal

export const isRefusal = (target: InsertTarget): target is InsertRefusal => 'refusal' in target

const childrenOf = (block: EmailBlock) => (block.children as EmailBlock[] | undefined) ?? []

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

// Where the library's "Insert" puts block, from the selected block. Walking up from the selection, the first level
// that takes the block decides: after that level's block in its parent, or, for the selection itself, at its end.
// A selection in the head, or none, puts it at the end of the body. Liquid refused at the level found is refused,
// never moved further up.
export function resolveInsertTarget(
  tree: EmailBlock,
  selectedId: string | null | undefined,
  block: Pick<EmailBlock, 'type' | 'children'>
): InsertTarget {
  const body = findBody(tree)
  const refused: InsertRefusal = { refusal: 'nesting', reason: nestingReason(block.type) }
  if (!body) return refused

  const decide = (place: InsertPlace): InsertTarget =>
    liquidPlacementError(tree, place.parentId, block) ? { refusal: 'liquid' } : place

  const path = selectedId ? pathTo(tree, selectedId) : null
  const bodyIndex = path ? path.findIndex((node) => node.id === body.id) : -1
  if (!path || bodyIndex === -1) {
    return takesFromLibrary(body, block) ? decide({ parentId: body.id, index: childrenOf(body).length }) : refused
  }

  const selected = path[path.length - 1]
  for (let i = path.length - 1; i >= bodyIndex; i--) {
    const node = path[i]
    const parent = i > bodyIndex ? path[i - 1] : null
    if (parent && takesFromLibrary(parent, block)) {
      return decide({ parentId: parent.id, index: childrenOf(parent).indexOf(node) + 1 })
    }
    if (node === selected && takesFromLibrary(node, block)) {
      return decide({ parentId: node.id, index: childrenOf(node).length })
    }
  }
  return refused
}
