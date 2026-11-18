import type { EmailBlock } from '../types'

// An update from a settings field: attribute keys, or `content` for the block's own content.
export type BlockUpdates = Record<string, unknown>

// The options of a write from the settings panel.
export interface UpdateOptions {
  // Consecutive writes with the same key make one undo step (a number being typed).
  coalesceKey?: string
}

// applyBlockUpdates returns a copy of block with updates applied: `content` sets the block's
// content, any other key an attribute. An undefined value removes the attribute (an undefined key
// is dropped anyway when the tree is serialised). A reset writes the inherited value instead (see
// panelFields.reset): the canvas falls back on the editor's defaults, the sent email on MJML's.
export function applyBlockUpdates(block: EmailBlock, updates: BlockUpdates): EmailBlock {
  const next = JSON.parse(JSON.stringify(block)) as EmailBlock
  const attributes = { ...((next.attributes as Record<string, unknown> | undefined) ?? {}) }
  Object.entries(updates).forEach(([key, value]) => {
    if (key === 'content') {
      ;(next as { content?: unknown }).content = value
    } else if (value === undefined) {
      delete attributes[key]
    } else {
      attributes[key] = value
    }
  })
  ;(next as { attributes?: unknown }).attributes = attributes
  return next
}
