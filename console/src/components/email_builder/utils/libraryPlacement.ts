import type { EmailBlock, MJMLComponentType } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'

// Where a block from the library may go. The blocks' own nesting rules (canAcceptChild), narrowed for sections and
// groups to columns and groups: the editor lets a section hold raw HTML and Liquid, which render as cells in its row
// of columns, but the library never puts them there. A filter on canAcceptChild, never an override: a group takes
// only columns, so it never takes a group.
export function takesFromLibrary(container: EmailBlock, block: Pick<EmailBlock, 'type'>): boolean {
  if (!EmailBlockClass.from(container).canAcceptChild(block.type)) return false
  if (container.type === 'mj-section' || container.type === 'mj-group') {
    return block.type === 'mj-column' || block.type === 'mj-group'
  }
  return true
}

// The kind of container a block that found no place needs: a column for content, a section for columns and groups,
// a social block for social elements. Sections, wrappers, raw HTML and Liquid always find the body.
export type NestingReason = 'column' | 'section' | 'social' | 'other'

const container = (type: MJMLComponentType) => ({ id: '', type }) as EmailBlock

export function nestingReason(type: MJMLComponentType): NestingReason {
  const block = { type }
  if (takesFromLibrary(container('mj-column'), block)) return 'column'
  if (takesFromLibrary(container('mj-section'), block)) return 'section'
  if (takesFromLibrary(container('mj-social'), block)) return 'social'
  return 'other'
}
