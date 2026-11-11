import type { EmailBlock } from '../types'

// Column widths of a section or a group, kept in proportion when a column comes or goes. Only mj-column children
// count: groups, raw HTML and Liquid keep their own attributes.

const SUM_TOLERANCE = 0.5

const columnsOf = (container: EmailBlock): EmailBlock[] =>
  container.type === 'mj-section' || container.type === 'mj-group'
    ? (container.children ?? []).filter((child) => child.type === 'mj-column')
    : []

const percentOf = (column: EmailBlock): number | null => {
  const width = (column.attributes as Record<string, unknown> | undefined)?.width
  if (typeof width !== 'string') return null
  const match = /^\s*(\d+(?:\.\d+)?)\s*%\s*$/.exec(width)
  return match ? Number(match[1]) : null
}

// Writes the exact shares, rounded to hundredths for every column but the last, which takes the remainder so the
// total is exactly 100
const write = (columns: EmailBlock[], shares: number[]) => {
  let used = 0
  columns.forEach((column, i) => {
    const hundredths = i === columns.length - 1 ? 10000 - used : Math.round(shares[i] * 100)
    used += hundredths
    if (!column.attributes) column.attributes = {}
    ;(column.attributes as Record<string, unknown>).width = `${hundredths / 100}%`
  })
}

const equal = (count: number) => Array.from({ length: count }, () => 100 / count)

/** After a column was added to (or moved into) the container: it gets its share, the others keep their ratio. */
export function rebalanceAfterAdd(container: EmailBlock, addedId: string): void {
  const columns = columnsOf(container)
  if (!columns.some((column) => column.id === addedId)) return
  const count = columns.length
  const others = columns.filter((column) => column.id !== addedId).map(percentOf)
  const sum = others.reduce<number>((total, width) => total + (width ?? 0), 0)
  const keepRatios =
    others.length > 0 &&
    others.every((width) => width !== null) &&
    Math.abs(sum - 100) <= SUM_TOLERANCE

  if (!keepRatios) {
    write(columns, equal(count))
    return
  }
  const share = 100 / count
  const scale = (100 - share) / sum
  write(
    columns,
    columns.map((column) => (column.id === addedId ? share : (percentOf(column) as number) * scale))
  )
}

/** After a column was removed from (or moved out of) the container: the others keep their ratio. */
export function rebalanceAfterRemove(container: EmailBlock): void {
  const columns = columnsOf(container)
  if (columns.length === 0) return
  const widths = columns.map(percentOf)
  const sum = widths.reduce<number>((total, width) => total + (width ?? 0), 0)
  if (widths.every((width) => width !== null) && sum > 0) {
    write(
      columns,
      widths.map((width) => ((width as number) * 100) / sum)
    )
  } else {
    write(columns, equal(columns.length))
  }
}
