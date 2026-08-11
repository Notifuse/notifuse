// Row-tree helpers for the explore drill-down.
//
// The table shows one dimension per level and fetches a level at a time: the
// root query groups by dimension 0, expanding a row queries dimensions 0..n+1
// filtered down to that row's path. Everything here is pure so the tree logic
// can be reasoned about (and tested) without a network or a React tree.

import { changePercent, toNumber } from './format'
import { SESSION_METRIC_KEYS, WebDimensionFilter } from './types'

/**
 * One row of the drill-down table.
 *
 * Dimension values are stored under their own name (`utm_source`, `device`, …)
 * for the whole ancestor path, because rebuilding the filters of a deeper level
 * needs every value above it, not just this row's own.
 */
export interface ExploreRow {
  /** Unique across the tree: the ancestor path joined with ':'. */
  key: string
  /** Index in the selected dimension list that this row is grouped by. */
  dimensionIndex: number
  childrenLoaded: boolean
  children?: ExploreRow[]
  isLoading?: boolean
  /** Every child fell below the session threshold, so none can be shown. */
  childrenFilteredByMinSessions?: boolean

  sessions: number
  median_duration: number
  bounce_rate: number
  median_scroll: number

  /** Dimension values, plus `<measure>_prev` / `<measure>_change` when comparing. */
  [field: string]: unknown
}

/** Period totals of the whole report, used for shares and the summary card. */
export interface ExploreTotals {
  sessions: number
  median_duration: number
  bounce_rate: number
  median_scroll: number
  sessions_change?: number
  median_duration_change?: number
  bounce_rate_change?: number
  median_scroll_change?: number
}

/** What to query to load the children of a row. */
export interface ChildrenQueryConfig {
  /** Dimension index the children are grouped by. */
  childDimensionIndex: number
  dimensionsToFetch: string[]
  filters: WebDimensionFilter[]
}

function isEmptyValue(value: unknown): boolean {
  return value === null || value === '' || value === undefined
}

/**
 * Key for a row inside the tree.
 *
 * Empty values all render as "(empty)" but are distinct rows as far as the
 * table is concerned, so the row index disambiguates them.
 */
export function generateRowKey(
  parentKey: string | null,
  dimensionValue: unknown,
  index?: number
): string {
  const value = isEmptyValue(dimensionValue)
    ? `[empty${index !== undefined ? `:${index}` : ''}]`
    : String(dimensionValue)
  return parentKey ? `${parentKey}:${value}` : value
}

/**
 * Dimensions and filters that load one row's children.
 *
 * The query groups by the whole path down to the child level and pins every
 * ancestor — including the row itself — to its value, so the engine returns
 * exactly the slice sitting under that row.
 */
export function calculateChildrenDimensionsAndFilters(
  row: ExploreRow,
  dimensions: string[],
  baseFilters: WebDimensionFilter[]
): ChildrenQueryConfig {
  const childDimensionIndex = row.dimensionIndex + 1
  const dimensionsToFetch = dimensions.slice(0, childDimensionIndex + 1)

  const drillDownFilters: WebDimensionFilter[] = dimensions
    .slice(0, childDimensionIndex)
    .map((dimension) => {
      const value = row[dimension]
      // A row grouped on a missing value is still a real row; it is selected by
      // "is empty" rather than by equality with an empty string.
      if (isEmptyValue(value)) {
        return { dimension, operator: 'isEmpty', values: [] }
      }
      return {
        dimension,
        operator: 'equals',
        values: [typeof value === 'number' ? value : String(value)]
      }
    })

  return {
    childDimensionIndex,
    dimensionsToFetch,
    filters: [...baseFilters, ...drillDownFilters]
  }
}

/**
 * Joins a level with the same level over the comparison period, adding
 * `<measure>_prev` to every current row. Rows that exist only in the
 * comparison period are dropped: the report lists what happened now, then says
 * how that moved.
 */
export function mergeComparisonData(
  // Nullable on purpose: the query API answers null, not [], for a breakdown
  // that matched nothing, and its declared type says otherwise.
  current: Record<string, unknown>[] | null | undefined,
  previous: Record<string, unknown>[] | undefined,
  dimension: string
): Record<string, unknown>[] {
  const previousByValue = new Map<string, Record<string, unknown>>()
  for (const row of previous ?? []) {
    previousByValue.set(String(row[dimension] ?? ''), row)
  }

  return (current ?? []).map((row) => {
    const match = previousByValue.get(String(row[dimension] ?? ''))
    const merged: Record<string, unknown> = { ...row }
    for (const measure of SESSION_METRIC_KEYS) {
      merged[`${measure}_prev`] = match?.[measure]
    }
    return merged
  })
}

/** Turns one level of engine rows into table rows below `parentKey`. */
export function buildExploreRows(
  apiRows: Record<string, unknown>[],
  dimensions: string[],
  dimensionIndex: number,
  parentKey: string | null,
  hasComparison: boolean
): ExploreRow[] {
  const dimension = dimensions[dimensionIndex]
  if (!dimension) return []

  return apiRows.map((raw, index) => {
    const dimensionValues: Record<string, unknown> = {}
    for (let level = 0; level <= dimensionIndex; level++) {
      const name = dimensions[level]
      if (name && raw[name] !== undefined) dimensionValues[name] = raw[name]
    }

    const row: ExploreRow = {
      ...dimensionValues,
      key: generateRowKey(parentKey, raw[dimension], index),
      dimensionIndex,
      childrenLoaded: false,
      sessions: toNumber(raw.sessions),
      median_duration: toNumber(raw.median_duration),
      bounce_rate: toNumber(raw.bounce_rate),
      median_scroll: toNumber(raw.median_scroll)
    }

    if (hasComparison) {
      for (const measure of SESSION_METRIC_KEYS) {
        const rawPrevious = raw[`${measure}_prev`]
        if (rawPrevious === undefined || rawPrevious === null) continue
        const previous = toNumber(rawPrevious)
        row[`${measure}_prev`] = previous
        // Growth from nothing has no percentage, so the delta stays unset
        // rather than rendering an infinite jump.
        if (previous > 0) {
          row[`${measure}_change`] = changePercent(toNumber(row[measure]), previous)
        }
      }
    }

    return row
  })
}

/** Whether there is still a dimension below this row to drill into. */
export function canExpandRow(row: ExploreRow, dimensions: string[]): boolean {
  return row.dimensionIndex < dimensions.length - 1
}

/** Attaches freshly loaded children to their parent, anywhere in the tree. */
export function insertChildrenIntoTree(
  rows: ExploreRow[],
  parentKey: string,
  children: ExploreRow[],
  childrenFilteredByMinSessions?: boolean
): ExploreRow[] {
  return rows.map((row) => {
    if (row.key === parentKey) {
      return {
        ...row,
        childrenLoaded: true,
        children,
        isLoading: false,
        childrenFilteredByMinSessions
      }
    }
    if (row.children) {
      return {
        ...row,
        children: insertChildrenIntoTree(
          row.children,
          parentKey,
          children,
          childrenFilteredByMinSessions
        )
      }
    }
    return row
  })
}

export function setRowLoading(
  rows: ExploreRow[],
  rowKey: string,
  isLoading: boolean
): ExploreRow[] {
  return rows.map((row) => {
    if (row.key === rowKey) return { ...row, isLoading }
    if (row.children) {
      return { ...row, children: setRowLoading(row.children, rowKey, isLoading) }
    }
    return row
  })
}

/**
 * Highest value of a measure anywhere in the tree.
 *
 * The heat map needs a ceiling and there is no endpoint that knows the global
 * one, so it is derived from what is on screen and raised as deeper levels
 * arrive.
 */
export function findMaxMeasure(rows: ExploreRow[], measure: string): number {
  let max = 0
  const visit = (items: ExploreRow[]) => {
    for (const row of items) {
      max = Math.max(max, toNumber(row[measure]))
      if (row.children) visit(row.children)
    }
  }
  visit(rows)
  return max
}

/**
 * Whether a result row was produced by a query over exactly these dimensions.
 *
 * React Query serves the previous result as placeholder data while a new key
 * is in flight, so a row can outlive the dimension list that produced it. Such
 * a row has no column at all for a dimension just added, and rendering that
 * gap would claim a blank value where there is only a pending one.
 *
 * Presence is tested with `in`, not truthiness: the engine selects every
 * grouped dimension, so a key that exists holding '' or null is a real empty
 * value and must count as covered.
 */
export function rowCoversDimensions(
  row: Record<string, unknown> | undefined,
  dimensions: string[]
): boolean {
  if (!row) return false
  return dimensions.every((dimension) => dimension in row)
}

/**
 * Whether a row lies on the path to the best-performing combination.
 *
 * A row at depth i carries a value for every dimension from 0 to i — the
 * drill-down groups by the whole prefix, not just the level — so matching all
 * of them marks the winner and every ancestor above it. That is what makes the
 * highlight read as a path down the tree rather than one stray row the reader
 * has to expand three levels to find.
 *
 * A dimension missing from `best` returns false rather than being skipped: an
 * absent key means the winning row came from a different dimension list, and
 * pointing at the wrong path is worse than pointing at none.
 */
export function isBestPathRow(
  row: ExploreRow,
  dimensions: string[],
  best: Record<string, unknown> | undefined
): boolean {
  if (!best) return false
  const covered = dimensions.slice(0, row.dimensionIndex + 1)
  if (covered.length === 0) return false

  return covered.every((dimension) => {
    if (!(dimension in best)) return false
    const bestValue = best[dimension]
    // "No value" arrives as null from one query and '' from another, so the
    // two have to compare equal or a winning empty branch never highlights.
    if (isEmptyValue(bestValue)) return isEmptyValue(row[dimension])
    return row[dimension] === bestValue
  })
}
