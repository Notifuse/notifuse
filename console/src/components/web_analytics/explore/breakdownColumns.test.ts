import { describe, expect, it } from 'vitest'
import { dimensionsForSchema, groupByCategory } from '../lib/dimensions'
import { toColumns, type CategoryGroup } from './breakdownColumns'

const group = (category: string, rows: number): CategoryGroup =>
  ({
    category,
    dimensions: Array.from({ length: rows }, (_, i) => ({ name: `${category}_${i}` }))
  }) as unknown as CategoryGroup

const height = (column: CategoryGroup[]) =>
  column.reduce((sum, entry) => sum + entry.dimensions.length + 2, 0)

describe('toColumns', () => {
  it('fills all four columns with the real catalog, keeping the categories in order', () => {
    const groups = groupByCategory(dimensionsForSchema('web_sessions'))
    const columns = toColumns(groups)
    expect(columns).toHaveLength(4)
    for (const column of columns) expect(column.length).toBeGreaterThan(0)
    expect(columns.flat().map((entry) => entry.category)).toEqual(groups.map((entry) => entry.category))
  })

  it('does not let one long category push the last column empty', () => {
    // Filling up to an average share put 2+5+4+12 rows in the first column and left the fourth empty.
    const columns = toColumns([
      group('Channel', 2),
      group('UTM', 5),
      group('Traffic', 4),
      group('Pages', 12),
      group('Device', 9),
      group('Geo', 5),
      group('Time', 7),
      group('Session', 3),
      group('User', 1),
      group('Custom', 2)
    ])
    expect(columns.every((column) => column.length > 0)).toBe(true)
    // No order-keeping split into four columns has a shorter tallest column (exhaustive check).
    const heights = columns.flat().map((entry) => entry.dimensions.length + 2)
    let bestTallest = Infinity
    for (let a = 1; a < heights.length; a++)
      for (let b = a + 1; b < heights.length; b++)
        for (let c = b + 1; c < heights.length; c++) {
          const sum = (from: number, to: number) => heights.slice(from, to).reduce((x, y) => x + y, 0)
          bestTallest = Math.min(
            bestTallest,
            Math.max(sum(0, a), sum(a, b), sum(b, c), sum(c, heights.length))
          )
        }
    expect(Math.max(...columns.map(height))).toBe(bestTallest)
  })

  it('puts the empty columns last when there are fewer categories than columns', () => {
    const columns = toColumns([group('Channel', 2), group('UTM', 5)])
    expect(columns.map((column) => column.map((entry) => entry.category))).toEqual([['Channel'], ['UTM'], [], []])
  })
})
