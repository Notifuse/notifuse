import type { DimensionCategory, DimensionInfo } from '../lib/dimensions'

const COLUMNS = 4

export interface CategoryGroup {
  category: DimensionCategory
  dimensions: DimensionInfo[]
}

// A category's title and its gap take about two rows.
const HEADER_ROWS = 2

/**
 * Spreads the categories over the grid columns without reordering them, choosing the cuts that
 * make the tallest column as short as possible. Filling each column up to an average share let
 * one long category (Pages) overflow the first column and shift the rest, leaving the last
 * column empty.
 */
export function toColumns(groups: CategoryGroup[], count = COLUMNS): CategoryGroup[][] {
  const heights = groups.map((group) => group.dimensions.length + HEADER_ROWS)
  const n = heights.length
  const prefix = [0]
  for (const height of heights) prefix.push(prefix[prefix.length - 1] + height)
  const span = (from: number, to: number) => prefix[to] - prefix[from]

  // best[k][i]: the tallest column when the first i categories fill k columns; cut[k][i]: where
  // the k-th column starts in that layout.
  const best: number[][] = Array.from({ length: count + 1 }, () => Array(n + 1).fill(Infinity))
  const cut: number[][] = Array.from({ length: count + 1 }, () => Array(n + 1).fill(0))
  best[0][0] = 0
  for (let k = 1; k <= count; k++) {
    for (let i = 0; i <= n; i++) {
      for (let j = 0; j <= i; j++) {
        const tallest = Math.max(best[k - 1][j], span(j, i))
        if (tallest < best[k][i]) {
          best[k][i] = tallest
          cut[k][i] = j
        }
      }
    }
  }

  const columns: CategoryGroup[][] = Array.from({ length: count }, () => [])
  let end = n
  for (let k = count; k >= 1; k--) {
    const start = cut[k][end]
    columns[k - 1] = groups.slice(start, end)
    end = start
  }
  // Empty columns go last, so fewer categories than columns still start on the left.
  return [...columns.filter((column) => column.length > 0), ...columns.filter((column) => column.length === 0)]
}
