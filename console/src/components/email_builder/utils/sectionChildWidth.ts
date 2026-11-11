import type { EmailBlock, MJMLComponentType } from '../types'

export type AttributeDefaults = Partial<Record<MJMLComponentType, Record<string, unknown>>>

export interface ParsedWidth {
  amount: number
  unit: 'px' | '%'
}

const isLayoutChild = (block: EmailBlock): boolean =>
  block.type === 'mj-column' || block.type === 'mj-group'

/** A width in pixels or percent, such as "60%" or "300px", or null when it is neither. */
export function parseWidth(value: unknown): ParsedWidth | null {
  if (typeof value !== 'string') return null
  const match = /^\s*(\d+(?:\.\d+)?)\s*(px|%)\s*$/.exec(value)
  return match ? { amount: Number(match[1]), unit: match[2] as ParsedWidth['unit'] } : null
}

const readableWidth = (value: unknown): string | undefined => {
  const width = parseWidth(value)
  return width ? `${width.amount}${width.unit}` : undefined
}

/** A group's own width, else the width the template's default attributes give groups, if either can be read. */
export function groupWidth(group: EmailBlock, attributeDefaults: AttributeDefaults = {}): string | undefined {
  return (
    readableWidth((group.attributes as Record<string, unknown> | undefined)?.width) ??
    readableWidth(attributeDefaults['mj-group']?.width)
  )
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
  const width =
    child.type === 'mj-group'
      ? groupWidth(child, attributeDefaults)
      : readableWidth((child.attributes as Record<string, unknown> | undefined)?.width)
  return width ?? sectionShare(section)
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
