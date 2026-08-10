import { describe, expect, it } from 'vitest'
import {
  buildExploreRows,
  calculateChildrenDimensionsAndFilters,
  canExpandRow,
  ExploreRow,
  findMaxMeasure,
  generateRowKey,
  insertChildrenIntoTree,
  mergeComparisonData,
  setRowLoading
} from './exploreRows'
import { WebDimensionFilter } from './types'

function row(overrides: Partial<ExploreRow> & { key: string; dimensionIndex: number }): ExploreRow {
  return {
    childrenLoaded: false,
    sessions: 0,
    median_duration: 0,
    bounce_rate: 0,
    median_scroll: 0,
    ...overrides
  }
}

describe('generateRowKey', () => {
  it('uses the value at the root and the parent path below it', () => {
    expect(generateRowKey(null, 'google')).toBe('google')
    expect(generateRowKey('google', 'cpc')).toBe('google:cpc')
  })

  it('keeps empty values distinct through the row index', () => {
    expect(generateRowKey(null, '', 0)).toBe('[empty:0]')
    expect(generateRowKey(null, null, 1)).toBe('[empty:1]')
    expect(generateRowKey('google', undefined, 2)).toBe('google:[empty:2]')
  })

  it('falls back to a bare empty marker without an index', () => {
    expect(generateRowKey(null, '')).toBe('[empty]')
  })

  it('keeps a zero value distinct from an empty one', () => {
    expect(generateRowKey(null, 0, 3)).toBe('0')
  })
})

describe('calculateChildrenDimensionsAndFilters', () => {
  const dimensions = ['utm_source', 'utm_medium', 'device']
  const base: WebDimensionFilter[] = [
    { dimension: 'country', operator: 'equals', values: ['FR'] }
  ]

  it('groups one level deeper and pins the row itself', () => {
    const config = calculateChildrenDimensionsAndFilters(
      row({ key: 'google', dimensionIndex: 0, utm_source: 'google' }),
      dimensions,
      base
    )

    expect(config.childDimensionIndex).toBe(1)
    expect(config.dimensionsToFetch).toEqual(['utm_source', 'utm_medium'])
    expect(config.filters).toEqual([
      { dimension: 'country', operator: 'equals', values: ['FR'] },
      { dimension: 'utm_source', operator: 'equals', values: ['google'] }
    ])
  })

  it('pins every ancestor of a deeper row', () => {
    const config = calculateChildrenDimensionsAndFilters(
      row({
        key: 'google:cpc',
        dimensionIndex: 1,
        utm_source: 'google',
        utm_medium: 'cpc'
      }),
      dimensions,
      []
    )

    expect(config.dimensionsToFetch).toEqual(['utm_source', 'utm_medium', 'device'])
    expect(config.filters).toEqual([
      { dimension: 'utm_source', operator: 'equals', values: ['google'] },
      { dimension: 'utm_medium', operator: 'equals', values: ['cpc'] }
    ])
  })

  it('selects a missing value with isEmpty rather than an equality', () => {
    const config = calculateChildrenDimensionsAndFilters(
      row({ key: '[empty:0]', dimensionIndex: 0, utm_source: '' }),
      dimensions,
      []
    )

    expect(config.filters).toEqual([
      { dimension: 'utm_source', operator: 'isEmpty', values: [] }
    ])
  })

  it('keeps numeric dimension values numeric', () => {
    const config = calculateChildrenDimensionsAndFilters(
      row({ key: '5', dimensionIndex: 0, day_of_week: 5 }),
      ['day_of_week', 'device'],
      []
    )

    expect(config.filters).toEqual([
      { dimension: 'day_of_week', operator: 'equals', values: [5] }
    ])
  })
})

describe('mergeComparisonData', () => {
  it('attaches the comparison measures of the matching value', () => {
    const merged = mergeComparisonData(
      [{ device: 'desktop', sessions: 120, median_duration: 40 }],
      [{ device: 'desktop', sessions: 100, median_duration: 30 }],
      'device'
    )

    expect(merged[0]).toMatchObject({
      device: 'desktop',
      sessions: 120,
      sessions_prev: 100,
      median_duration_prev: 30
    })
  })

  it('leaves the comparison undefined when the value is new', () => {
    const merged = mergeComparisonData(
      [{ device: 'tablet', sessions: 5 }],
      [{ device: 'desktop', sessions: 100 }],
      'device'
    )

    expect(merged[0].sessions_prev).toBeUndefined()
  })

  it('drops values that only exist in the comparison period', () => {
    const merged = mergeComparisonData(
      [{ device: 'desktop', sessions: 5 }],
      [
        { device: 'desktop', sessions: 4 },
        { device: 'mobile', sessions: 90 }
      ],
      'device'
    )

    expect(merged).toHaveLength(1)
  })

  it('matches empty and null as the same value', () => {
    const merged = mergeComparisonData(
      [{ utm_source: '', sessions: 10 }],
      [{ utm_source: null, sessions: 8 }],
      'utm_source'
    )

    expect(merged[0].sessions_prev).toBe(8)
  })

  it('tolerates a missing comparison response', () => {
    const merged = mergeComparisonData([{ device: 'desktop', sessions: 5 }], undefined, 'device')
    expect(merged[0].sessions_prev).toBeUndefined()
  })
})

describe('buildExploreRows', () => {
  const dimensions = ['utm_source', 'device']

  it('reads measures that arrive as strings', () => {
    const rows = buildExploreRows(
      [{ utm_source: 'google', sessions: '120', median_duration: '42.5', bounce_rate: '31.2' }],
      dimensions,
      0,
      null,
      false
    )

    expect(rows[0]).toMatchObject({
      key: 'google',
      dimensionIndex: 0,
      childrenLoaded: false,
      sessions: 120,
      median_duration: 42.5,
      bounce_rate: 31.2,
      median_scroll: 0
    })
  })

  it('keeps every ancestor value so deeper levels can be filtered', () => {
    const rows = buildExploreRows(
      [{ utm_source: 'google', device: 'mobile', sessions: 10 }],
      dimensions,
      1,
      'google',
      false
    )

    expect(rows[0].utm_source).toBe('google')
    expect(rows[0].device).toBe('mobile')
    expect(rows[0].key).toBe('google:mobile')
  })

  it('derives the change from the comparison measures', () => {
    const rows = buildExploreRows(
      [{ utm_source: 'google', sessions: 120, sessions_prev: 100 }],
      dimensions,
      0,
      null,
      true
    )

    expect(rows[0].sessions_prev).toBe(100)
    expect(rows[0].sessions_change).toBeCloseTo(20)
  })

  it('leaves the change unset when there is nothing to grow from', () => {
    const rows = buildExploreRows(
      [{ utm_source: 'google', sessions: 120, sessions_prev: 0 }],
      dimensions,
      0,
      null,
      true
    )

    expect(rows[0].sessions_change).toBeUndefined()
  })

  it('ignores comparison measures when not comparing', () => {
    const rows = buildExploreRows(
      [{ utm_source: 'google', sessions: 120, sessions_prev: 100 }],
      dimensions,
      0,
      null,
      false
    )

    expect(rows[0].sessions_prev).toBeUndefined()
  })

  it('returns nothing when the level has no dimension', () => {
    expect(buildExploreRows([{ sessions: 1 }], dimensions, 5, null, false)).toEqual([])
  })
})

describe('canExpandRow', () => {
  it('is true while a dimension remains below the row', () => {
    expect(canExpandRow(row({ key: 'a', dimensionIndex: 0 }), ['a', 'b'])).toBe(true)
  })

  it('is false on the deepest level', () => {
    expect(canExpandRow(row({ key: 'a', dimensionIndex: 1 }), ['a', 'b'])).toBe(false)
  })
})

describe('insertChildrenIntoTree', () => {
  const tree: ExploreRow[] = [
    row({
      key: 'google',
      dimensionIndex: 0,
      childrenLoaded: true,
      children: [row({ key: 'google:cpc', dimensionIndex: 1, isLoading: true })]
    }),
    row({ key: 'direct', dimensionIndex: 0 })
  ]

  it('attaches children to a nested parent and clears its loading flag', () => {
    const child = row({ key: 'google:cpc:mobile', dimensionIndex: 2 })
    const next = insertChildrenIntoTree(tree, 'google:cpc', [child])

    const parent = next[0].children?.[0]
    expect(parent?.children).toEqual([child])
    expect(parent?.childrenLoaded).toBe(true)
    expect(parent?.isLoading).toBe(false)
  })

  it('records that the threshold hid every child', () => {
    const next = insertChildrenIntoTree(tree, 'direct', [], true)
    expect(next[1].childrenFilteredByMinSessions).toBe(true)
    expect(next[1].children).toEqual([])
  })

  it('leaves the original tree untouched', () => {
    insertChildrenIntoTree(tree, 'direct', [row({ key: 'x', dimensionIndex: 1 })])
    expect(tree[1].children).toBeUndefined()
  })

  it('returns the tree unchanged when the parent is gone', () => {
    const next = insertChildrenIntoTree(tree, 'missing', [])
    expect(next[0].children).toEqual(tree[0].children)
    expect(next[1].childrenLoaded).toBe(false)
  })
})

describe('setRowLoading', () => {
  it('flags a nested row without touching its siblings', () => {
    const tree: ExploreRow[] = [
      row({
        key: 'google',
        dimensionIndex: 0,
        children: [row({ key: 'google:cpc', dimensionIndex: 1 })]
      }),
      row({ key: 'direct', dimensionIndex: 0 })
    ]

    const next = setRowLoading(tree, 'google:cpc', true)
    expect(next[0].children?.[0].isLoading).toBe(true)
    expect(next[1].isLoading).toBeUndefined()
  })
})

describe('findMaxMeasure', () => {
  it('looks through the whole tree, not just the top level', () => {
    const tree: ExploreRow[] = [
      row({
        key: 'google',
        dimensionIndex: 0,
        median_duration: 30,
        children: [row({ key: 'google:cpc', dimensionIndex: 1, median_duration: 95 })]
      }),
      row({ key: 'direct', dimensionIndex: 0, median_duration: 42 })
    ]

    expect(findMaxMeasure(tree, 'median_duration')).toBe(95)
  })

  it('is zero on an empty report', () => {
    expect(findMaxMeasure([], 'median_duration')).toBe(0)
  })
})
