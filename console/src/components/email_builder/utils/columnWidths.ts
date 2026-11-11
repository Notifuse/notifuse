import type { EmailBlock } from '../types'
import { groupWidth, parseWidth, type AttributeDefaults } from './sectionChildWidth'

// Column widths of a section or a group, kept in proportion when a column or a group comes or goes. In a section,
// the groups take their part first, as in the email: their percentage width (their own, else the one the template's
// default attributes give groups), or an equal share of the section when they have none (a pixel width counts as
// that share too). The columns share what is left. Groups, raw HTML and
// Liquid keep their own attributes: only column widths are written.

const SUM_TOLERANCE = 0.5

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

// What the groups of a section leave to its columns, the section holding `layoutCount` columns and groups, or null
// when they leave no room
const roomLeft = (
  container: EmailBlock,
  groups: EmailBlock[],
  layoutCount: number,
  attributeDefaults: AttributeDefaults
): number | null => {
  if (container.type !== 'mj-section' || groups.length === 0) return 100
  const share = 100 / Math.max(layoutCount, 1)
  const taken = groups.reduce(
    (total, group) => total + (percent(groupWidth(group, attributeDefaults)) ?? share),
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
  attributeDefaults: AttributeDefaults
): number => roomLeft(container, groups, layoutCount, attributeDefaults) ?? 100

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
  attributeDefaults: AttributeDefaults = {}
): void {
  const columns = columnsOf(container)
  if (columns.length === 0) return
  const groups = container.type === 'mj-section' ? childrenOf(container, 'mj-group') : []
  const layoutCount = columns.length + groups.length

  if (groups.some((group) => group.id === addedId)) {
    // A group that leaves the columns no room leaves them as they are
    const room = roomLeft(container, groups, layoutCount, attributeDefaults)
    if (room !== null) scaleTo(columns, room)
    return
  }
  if (!columns.some((column) => column.id === addedId)) return
  const budget = columnBudget(container, groups, layoutCount, attributeDefaults)

  // What the other columns filled before this one came
  const previousBudget = columnBudget(container, groups, layoutCount - 1, attributeDefaults)
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

/** After a column or a group was removed from (or moved out of) the container: the columns keep their ratio. */
export function rebalanceAfterRemove(
  container: EmailBlock,
  attributeDefaults: AttributeDefaults = {}
): void {
  const columns = columnsOf(container)
  if (columns.length === 0) return
  const groups = container.type === 'mj-section' ? childrenOf(container, 'mj-group') : []
  scaleTo(columns, columnBudget(container, groups, columns.length + groups.length, attributeDefaults))
}
