import type { EmailBlock } from '../types'
import { MJ_SECTION_DEFAULTS, MJ_WRAPPER_DEFAULTS } from '../mjml-defaults'
import { groupWidth, parseWidth, type AttributeDefaults } from './sectionChildWidth'

// Column widths of a section or a group, kept in proportion when a column or a group comes or goes. In a section,
// the groups take their part first, as in the email: their width (their own, else the one the template's default
// attributes give groups), a pixel width counting as its part of the body's width less the section's side padding,
// or an equal share of the section when they have none. The columns share what is left. Groups, raw HTML and
// Liquid keep their own attributes: only column widths are written.

const SUM_TOLERANCE = 0.5

/** MJML's default body width, in pixels */
export const DEFAULT_BODY_WIDTH = 600

export interface LayoutOptions {
  /** The email body's width in pixels, which a group's pixel width is measured against */
  bodyWidth?: number
}

/** The body's width in pixels, as the email gives it, else MJML's default. */
export function bodyWidthOf(tree: EmailBlock): number {
  const body = tree.type === 'mj-body' ? tree : tree.children?.find((child) => child.type === 'mj-body')
  const width = parseWidth((body?.attributes as Record<string, unknown> | undefined)?.width)
  return width?.unit === 'px' && width.amount > 0 ? width.amount : DEFAULT_BODY_WIDTH
}

const childrenOf = (container: EmailBlock, type: string): EmailBlock[] =>
  (container.children ?? []).filter((child) => child.type === type)

const columnsOf = (container: EmailBlock): EmailBlock[] =>
  container.type === 'mj-section' || container.type === 'mj-group'
    ? childrenOf(container, 'mj-column')
    : []

const percent = (value: unknown): number | null => {
  const width = parseWidth(value)
  return width?.unit === '%' ? width.amount : null
}

const percentOf = (block: EmailBlock): number | null =>
  percent((block.attributes as Record<string, unknown> | undefined)?.width)

const pixels = (value: unknown): number | null => {
  const width = parseWidth(value)
  return width?.unit === 'px' ? width.amount : null
}

// One side of a CSS padding shorthand ("20px", "20px 0", "10px 20px 30px", "10px 20px 30px 40px")
const shorthandSide = (value: unknown, side: 'left' | 'right'): unknown => {
  if (typeof value !== 'string') return undefined
  const parts = value.trim().split(/\s+/)
  if (parts.length === 1) return parts[0]
  if (side === 'right' || parts.length < 4) return parts[1]
  return parts[3]
}

// A side padding of the section, as MJML reads it: its own attributes, then the template's default attributes,
// each with its padding-left or padding-right before its padding shorthand
const sidePadding = (
  section: EmailBlock,
  attributeDefaults: AttributeDefaults,
  side: 'left' | 'right',
  type: 'mj-section' | 'mj-wrapper' = 'mj-section'
): number => {
  const key = side === 'left' ? 'paddingLeft' : 'paddingRight'
  const sources = [
    section.attributes as Record<string, unknown> | undefined,
    attributeDefaults[type],
    (type === 'mj-section' ? MJ_SECTION_DEFAULTS : MJ_WRAPPER_DEFAULTS) as Record<string, unknown>
  ]
  for (const source of sources) {
    const value = source?.[key] ?? shorthandSide(source?.padding, side)
    if (value !== undefined && value !== '') return pixels(value) ?? 0
  }
  return 0
}

/** The section's inner width in pixels, which its columns and its gutter are measured against: the body's width less
 * the section's side padding. */
export function sectionInnerWidth(
  section: EmailBlock,
  attributeDefaults: AttributeDefaults,
  bodyWidth: number
): number {
  return bodyWidth - sidePadding(section, attributeDefaults, 'left') - sidePadding(section, attributeDefaults, 'right')
}

// A side border's width, as mjml-core's borderParser reads it: the first whole number of border-left or
// border-right, else of border
const sideBorder = (
  block: EmailBlock,
  attributeDefaults: AttributeDefaults,
  side: 'left' | 'right',
  type: 'mj-section' | 'mj-wrapper'
): number => {
  const key = side === 'left' ? 'borderLeft' : 'borderRight'
  const sources = [block.attributes as Record<string, unknown> | undefined, attributeDefaults[type]]
  const pick = (name: string): unknown => sources.find((source) => source?.[name])?.[name]
  const value = pick(key) || pick('border')
  const match = typeof value === 'string' ? /(?:^| )(\d+)/.exec(value) : null
  return match ? parseInt(match[1], 10) : 0
}

/**
 * The width a section's or a wrapper's children are laid out in, as MJML's getBoxWidths: the width it is given
 * less its side padding and side borders.
 */
export function boxWidth(
  block: EmailBlock,
  type: 'mj-section' | 'mj-wrapper',
  attributeDefaults: AttributeDefaults,
  outerWidth: number
): number {
  return (
    outerWidth -
    sidePadding(block, attributeDefaults, 'left', type) -
    sidePadding(block, attributeDefaults, 'right', type) -
    sideBorder(block, attributeDefaults, 'left', type) -
    sideBorder(block, attributeDefaults, 'right', type)
  )
}

// A group's part of the section in percent, or null when it has no width that can be read
const groupPercent = (
  section: EmailBlock,
  group: EmailBlock,
  attributeDefaults: AttributeDefaults,
  bodyWidth: number
): number | null => {
  const width = parseWidth(groupWidth(group, attributeDefaults))
  if (!width) return null
  if (width.unit === '%') return width.amount
  const inner = sectionInnerWidth(section, attributeDefaults, bodyWidth)
  return inner > 0 ? (width.amount * 100) / inner : null
}

// What the groups of a section leave to its columns, the section holding `layoutCount` columns and groups, or null
// when they leave no room
const roomLeft = (
  container: EmailBlock,
  groups: EmailBlock[],
  layoutCount: number,
  attributeDefaults: AttributeDefaults,
  bodyWidth: number
): number | null => {
  if (container.type !== 'mj-section' || groups.length === 0) return 100
  const share = 100 / Math.max(layoutCount, 1)
  const taken = groups.reduce(
    (total, group) => total + (groupPercent(container, group, attributeDefaults, bodyWidth) ?? share),
    0
  )
  const left = 100 - taken
  return left > 0 ? left : null
}

// Groups that leave no room are not counted, as before groups were counted at all
const columnBudget = (
  container: EmailBlock,
  groups: EmailBlock[],
  layoutCount: number,
  attributeDefaults: AttributeDefaults,
  bodyWidth: number
): number => roomLeft(container, groups, layoutCount, attributeDefaults, bodyWidth) ?? 100

/**
 * How far the columns and groups of a section go past its width, in percent, or 0 when they fit. A column or group
 * without a width takes its share of the section, as in the email; a column in pixels is not counted.
 */
export function sectionOverflow(
  section: EmailBlock,
  attributeDefaults: AttributeDefaults = {},
  { bodyWidth = DEFAULT_BODY_WIDTH }: LayoutOptions = {}
): number {
  if (section.type !== 'mj-section') return 0
  const layout = (section.children ?? []).filter((child) => child.type === 'mj-column' || child.type === 'mj-group')
  const share = 100 / Math.max(layout.length, 1)
  const total = layout.reduce((sum, child) => {
    if (child.type === 'mj-group') return sum + (groupPercent(section, child, attributeDefaults, bodyWidth) ?? share)
    const own = (child.attributes as Record<string, unknown> | undefined)?.width
    return sum + (parseWidth(own) ? (percent(own) ?? 0) : share)
  }, 0)
  const over = total - 100
  return over > SUM_TOLERANCE ? Math.round(over * 100) / 100 : 0
}

// Writes the exact shares, rounded to hundredths for every column but the last, which takes the remainder so the
// total is exactly the budget
const write = (columns: EmailBlock[], shares: number[], budget: number) => {
  const total = Math.round(budget * 100)
  let used = 0
  columns.forEach((column, i) => {
    const hundredths = i === columns.length - 1 ? total - used : Math.round(shares[i] * 100)
    used += hundredths
    if (!column.attributes) column.attributes = {}
    ;(column.attributes as Record<string, unknown>).width = `${hundredths / 100}%`
  })
}

const equal = (count: number, budget: number) => Array.from({ length: count }, () => budget / count)

// The columns keep their ratio, scaled to the budget, or share it equally when their widths are not all
// percentages
const scaleTo = (columns: EmailBlock[], budget: number) => {
  const widths = columns.map(percentOf)
  const sum = widths.reduce<number>((total, width) => total + (width ?? 0), 0)
  if (widths.every((width) => width !== null) && sum > 0) {
    write(
      columns,
      widths.map((width) => ((width as number) * budget) / sum),
      budget
    )
  } else {
    write(columns, equal(columns.length, budget), budget)
  }
}

/**
 * After a column or a group was added to (or moved into) the container. A new column gets its share of what the
 * groups leave and the other columns keep their ratio; a new group makes room, the columns keeping their ratio.
 */
export function rebalanceAfterAdd(
  container: EmailBlock,
  addedId: string,
  attributeDefaults: AttributeDefaults = {},
  { bodyWidth = DEFAULT_BODY_WIDTH }: LayoutOptions = {}
): void {
  const columns = columnsOf(container)
  if (columns.length === 0) return
  const groups = container.type === 'mj-section' ? childrenOf(container, 'mj-group') : []
  const layoutCount = columns.length + groups.length

  if (groups.some((group) => group.id === addedId)) {
    // A group that leaves the columns no room leaves them as they are
    const room = roomLeft(container, groups, layoutCount, attributeDefaults, bodyWidth)
    if (room !== null) scaleTo(columns, room)
    return
  }
  if (!columns.some((column) => column.id === addedId)) return
  const budget = columnBudget(container, groups, layoutCount, attributeDefaults, bodyWidth)

  // What the other columns filled before this one came
  const previousBudget = columnBudget(container, groups, layoutCount - 1, attributeDefaults, bodyWidth)
  const others = columns.filter((column) => column.id !== addedId).map(percentOf)
  const sum = others.reduce<number>((total, width) => total + (width ?? 0), 0)
  const keepRatios =
    others.length > 0 &&
    others.every((width) => width !== null) &&
    Math.abs(sum - previousBudget) <= SUM_TOLERANCE

  if (!keepRatios) {
    write(columns, equal(columns.length, budget), budget)
    return
  }
  const share = budget / columns.length
  const scale = (budget - share) / sum
  write(
    columns,
    columns.map((column) => (column.id === addedId ? share : (percentOf(column) as number) * scale)),
    budget
  )
}

export interface RemoveOptions extends LayoutOptions {
  /** What went: a group whose removal still leaves the columns no room leaves them as they are, as on add */
  removed?: 'mj-column' | 'mj-group'
}

/** After a column or a group was removed from (or moved out of) the container: the columns keep their ratio. */
export function rebalanceAfterRemove(
  container: EmailBlock,
  attributeDefaults: AttributeDefaults = {},
  { bodyWidth = DEFAULT_BODY_WIDTH, removed = 'mj-column' }: RemoveOptions = {}
): void {
  const columns = columnsOf(container)
  if (columns.length === 0) return
  const groups = container.type === 'mj-section' ? childrenOf(container, 'mj-group') : []
  const layoutCount = columns.length + groups.length
  const room = roomLeft(container, groups, layoutCount, attributeDefaults, bodyWidth)
  if (room === null && removed === 'mj-group') return
  scaleTo(columns, room ?? 100)
}
