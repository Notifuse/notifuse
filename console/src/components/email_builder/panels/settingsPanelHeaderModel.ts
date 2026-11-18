import type { EmailBlock } from '../types'

/** The blocks from the email's body (or head) down to the block, root excluded; [] when not found. */
export function blockPath(tree: EmailBlock, blockId: string): EmailBlock[] {
  const walk = (block: EmailBlock, trail: EmailBlock[]): EmailBlock[] | null => {
    const here = block.type === 'mjml' ? trail : [...trail, block]
    if (block.id === blockId) return here
    for (const child of block.children ?? []) {
      const found = walk(child, here)
      if (found) return found
    }
    return null
  }
  return walk(tree, []) ?? []
}

export interface HeaderActions {
  clone: boolean
  save: boolean
  remove: boolean
}

/**
 * What the header offers for the block, as the canvas and the tree do: a block of the body can be
 * cloned, saved to the library and deleted; a head block other than the default attributes can be
 * deleted; the email, its head, its body and its default attributes cannot.
 */
export function headerActions(path: EmailBlock[]): HeaderActions {
  const none = { clone: false, save: false, remove: false }
  const block = path.at(-1)
  if (!block || path.length < 2) return none
  if (path[0].type === 'mj-body') return { clone: true, save: true, remove: true }
  if (path[0].type === 'mj-head' && path.length === 2 && block.type !== 'mj-attributes') {
    return { ...none, remove: true }
  }
  return none
}

export type HeaderTone = 'content' | 'structure' | 'head' | 'code'

const STRUCTURE = new Set(['mj-body', 'mj-wrapper', 'mj-section', 'mj-column', 'mj-group'])
const CODE = new Set(['mj-raw', 'mj-liquid'])

/** The colour of the block's icon tile: content, structure, head or code. */
export function headerTone(path: EmailBlock[]): HeaderTone {
  const block = path.at(-1)
  if (!block) return 'content'
  if (CODE.has(block.type)) return 'code'
  if (path[0]?.type === 'mj-head' || block.type === 'mj-head') return 'head'
  if (STRUCTURE.has(block.type)) return 'structure'
  return 'content'
}
