import type { EmailBlock, MJMLComponentType } from '../types'

type AttributeDefaults = Partial<Record<MJMLComponentType, Record<string, unknown>>>

const isLayoutChild = (block: EmailBlock): boolean =>
  block.type === 'mj-column' || block.type === 'mj-group'

const readableWidth = (value: unknown): string | undefined => {
  if (typeof value !== 'string') return undefined
  const width = value.trim()
  return /^\d+(\.\d+)?(px|%)$/.test(width) ? width : undefined
}

/**
 * Width a column or a group without one takes in its section, as MJML computes
 * it: the section split between its column and group children. Raw HTML and
 * Liquid blocks do not count (what a Liquid block produces is only known at
 * send time).
 */
export function sectionShare(section: EmailBlock): string {
  const count = (section.children ?? []).filter(isLayoutChild).length
  return `${Number((100 / Math.max(count, 1)).toFixed(2))}%`
}

/**
 * Width of a section child on the canvas, as in the email: its own width, then,
 * for a group, the width of the template's default attributes, then its share
 * of the section. Raw HTML and Liquid blocks get none.
 */
export function sectionChildWidth(
  child: EmailBlock,
  section: EmailBlock,
  attributeDefaults: AttributeDefaults = {}
): string | undefined {
  if (!isLayoutChild(child)) return undefined
  const own = readableWidth((child.attributes as Record<string, unknown> | undefined)?.width)
  if (own) return own
  if (child.type === 'mj-group') {
    const fromDefaults = readableWidth(attributeDefaults['mj-group']?.width)
    if (fromDefaults) return fromDefaults
  }
  return sectionShare(section)
}

/** The section that directly holds the block, or null when its parent is not a section. */
export function findParentSection(tree: EmailBlock, blockId: string): EmailBlock | null {
  const children = (tree.children ?? []) as EmailBlock[]
  if (tree.type === 'mj-section' && children.some((child) => child.id === blockId)) {
    return tree
  }
  for (const child of children) {
    const found = findParentSection(child, blockId)
    if (found) return found
  }
  return null
}
