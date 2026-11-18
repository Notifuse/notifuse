import type { EmailBlock } from '../types'
import { parseWidth, type AttributeDefaults } from './sectionChildWidth'
import { bodyWidthOf, boxWidth } from './columnWidths'

// A section's gutter on the canvas, as MJML 5.4.1 lays it out (mjml-column: getDesktopWidth,
// getDesktopPaddingValues): the section passes its gutter to its columns and to the columns of its groups. A
// column among n children of its parent loses gutter × (n − 1) / n of its width and gets half the gutter as
// padding on each side that faces another child; the outer edges get none. Widths and the gutter are read in the
// column's unit, a px gutter turned into % (and back) against the parent's width. Only columns get it: a group
// in a section keeps its width and takes no padding.

export interface GutterCell {
  /** Width of the column's cell, its padding included (the cell is laid out as border-box) */
  width: string
  paddingLeft: string
  paddingRight: string
}

// JavaScript's Number(parseFloat(value).toFixed(6)), as MJML normalizes widths
const round6 = (value: number): number => Number(value.toFixed(6))

const formatLength = (value: number, unit: 'px' | '%'): string =>
  unit === 'px' ? `${Math.round(value)}px` : `${round6(value)}%`

/**
 * The cell of each child of a section or a group under a gutter, in the children's order: null for a child that
 * takes none (not a column), and null for every child when there is no gutter that can be read.
 *
 * @param children every child of the parent, raw HTML and Liquid included: MJML counts them as siblings
 * @param widthOf the width the child has without a gutter ("50%", "300px"), as the canvas gives it
 * @param containerWidth the parent's inner width in pixels, which a gutter is converted against
 */
export function gutterCells(
  children: EmailBlock[],
  gutter: unknown,
  containerWidth: number,
  direction: string | undefined,
  widthOf: (child: EmailBlock) => string | undefined
): (GutterCell | null)[] {
  // A number without a unit is read in px, as MJML's width parser reads it
  const parsedGutter = parseWidth(
    typeof gutter === 'string' && /^\s*\d+(?:\.\d+)?\s*$/.test(gutter) ? `${gutter}px` : gutter
  )
  if (!parsedGutter || !(containerWidth > 0)) return children.map(() => null)

  const sibling = children.length
  const isRTL = direction === 'rtl'

  const gutterIn = (unit: 'px' | '%'): number => {
    if (parsedGutter.unit === unit) return parsedGutter.amount
    return unit === '%' ? (parsedGutter.amount / containerWidth) * 100 : (containerWidth * parsedGutter.amount) / 100
  }

  return children.map((child, index) => {
    if (child.type !== 'mj-column') return null
    const width = parseWidth(widthOf(child))
    if (!width) return null
    const { unit } = width

    const gutterValue = gutterIn(unit)
    const reduction = (gutterValue * (sibling - 1)) / sibling
    let reduced = Math.max(0, round6(width.amount - reduction))
    if (unit === 'px') {
      // The pixels the floor drops are given back one each to the first columns
      const floorWidth = Math.floor(reduced)
      const extraPixels = Math.max(0, Math.min(sibling, Math.round(sibling * (reduced - floorWidth))))
      reduced = floorWidth + (index < extraPixels ? 1 : 0)
    }

    let left = 0
    let right = 0
    if (sibling > 1) {
      const normalized = unit === 'px' ? Math.round(gutterValue) : gutterValue
      const leading = unit === 'px' ? Math.ceil(normalized / 2) : normalized / 2
      const trailing = unit === 'px' ? Math.floor(normalized / 2) : normalized / 2
      const first = index === 0
      const last = index === sibling - 1
      if (isRTL) {
        right = first ? 0 : trailing
        left = last ? 0 : leading
      } else {
        right = last ? 0 : leading
        left = first ? 0 : trailing
      }
    }

    return {
      width: formatLength(reduced + left + right, unit),
      paddingLeft: formatLength(left, unit),
      paddingRight: formatLength(right, unit)
    }
  })
}

// The block that directly holds the one with this id
const parentOf = (tree: EmailBlock, id: string): EmailBlock | null => {
  for (const child of tree.children ?? []) {
    if (child.id === id) return tree
    const found = parentOf(child, id)
    if (found) return found
  }
  return null
}

/**
 * The width a section's gutter is measured against, in pixels, as MJML's column reads it: the section's box, in the
 * box of the mj-wrapper that holds it if any, else in the body's width.
 */
export function sectionGutterWidth(
  tree: EmailBlock,
  section: EmailBlock,
  attributeDefaults: AttributeDefaults
): number {
  const body = bodyWidthOf(tree)
  const parent = parentOf(tree, section.id)
  const outer = parent?.type === 'mj-wrapper' ? boxWidth(parent, 'mj-wrapper', attributeDefaults, body) : body
  return boxWidth(section, 'mj-section', attributeDefaults, outer)
}
