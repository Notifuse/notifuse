import { describe, it, expect } from 'vitest'
import { asEditableRoot, isFilterComplete, isFilterRowComplete, isTreeQueryable } from './tree_completeness'
import type { DimensionFilter } from '../../services/api/segment'
import type { TreeNode, TreeNodeLeaf } from '../../services/api/segment'

const branchOf = (...leaves: TreeNodeLeaf[]): TreeNode => ({
  kind: 'branch',
  branch: {
    operator: 'and',
    leaves: leaves.map((leaf) => ({ kind: 'leaf', leaf }) as TreeNode)
  }
})

const listLeaf: TreeNodeLeaf = {
  source: 'contact_lists',
  contact_list: { operator: 'in', list_id: 'newsletter', status: 'active' }
}

const timelineLeaf: TreeNodeLeaf = {
  source: 'contact_timeline',
  contact_timeline: {
    kind: 'email.opened',
    count_operator: 'at_least',
    count_value: 1,
    timeframe_operator: 'anytime',
    timeframe_values: []
  }
}

const goalLeaf: TreeNodeLeaf = {
  source: 'custom_events_goals',
  custom_events_goal: {
    goal_type: 'purchase',
    aggregate_operator: 'count',
    operator: 'gte',
    value: 1,
    timeframe_operator: 'anytime',
    timeframe_values: []
  }
}

const contactLeaf: TreeNodeLeaf = {
  source: 'contacts',
  contact: {
    filters: [
      { field_name: 'email', field_type: 'string', operator: 'contains', string_values: ['@acme'] }
    ]
  }
}

describe('isTreeQueryable', () => {
  it('accepts a complete tree of every condition type', () => {
    expect(isTreeQueryable(branchOf(listLeaf, timelineLeaf, goalLeaf, contactLeaf))).toBe(true)
  })

  it('rejects an empty branch', () => {
    expect(isTreeQueryable({ kind: 'branch', branch: { operator: 'and', leaves: [] } })).toBe(false)
  })

  it('rejects nothing at all', () => {
    expect(isTreeQueryable(undefined)).toBe(false)
    expect(isTreeQueryable(null)).toBe(false)
  })

  it('rejects a tree where one of several conditions is still incomplete', () => {
    const incomplete: TreeNodeLeaf = {
      source: 'contact_lists',
      contact_list: { operator: 'in', list_id: '' }
    }
    expect(isTreeQueryable(branchOf(listLeaf, incomplete))).toBe(false)
  })

  it('accepts nested branches', () => {
    const nested: TreeNode = {
      kind: 'branch',
      branch: { operator: 'or', leaves: [branchOf(listLeaf), branchOf(goalLeaf)] }
    }
    expect(isTreeQueryable(nested)).toBe(true)
  })
})

describe('isTreeQueryable — conditions being filled in', () => {
  it('rejects a freshly added list condition with no list picked', () => {
    const fresh: TreeNodeLeaf = {
      source: 'contact_lists',
      contact_list: { operator: 'in', list_id: '', status: undefined }
    }
    expect(isTreeQueryable(branchOf(fresh))).toBe(false)
  })

  it('accepts a list condition as soon as the list is picked', () => {
    const noStatus: TreeNodeLeaf = {
      source: 'contact_lists',
      contact_list: { operator: 'in', list_id: 'newsletter' }
    }
    expect(isTreeQueryable(branchOf(noStatus))).toBe(true)
  })

  it('rejects a freshly added activity condition with no event kind', () => {
    const fresh: TreeNodeLeaf = {
      source: 'contact_timeline',
      contact_timeline: {
        kind: '',
        count_operator: 'at_least',
        count_value: 1,
        timeframe_operator: 'anytime',
        timeframe_values: []
      }
    }
    expect(isTreeQueryable(branchOf(fresh))).toBe(false)
  })

  it('rejects a contact condition with no filter yet', () => {
    const fresh: TreeNodeLeaf = { source: 'contacts', contact: { filters: [] } }
    expect(isTreeQueryable(branchOf(fresh))).toBe(false)
  })

  it('rejects a count the user has cleared', () => {
    const cleared = {
      ...timelineLeaf,
      contact_timeline: {
        ...timelineLeaf.contact_timeline!,
        count_value: null as unknown as number
      }
    }
    expect(isTreeQueryable(branchOf(cleared))).toBe(false)
  })

  it('rejects a "between" goal missing its second value', () => {
    const between = {
      ...goalLeaf,
      custom_events_goal: { ...goalLeaf.custom_events_goal!, operator: 'between' as const }
    }
    expect(isTreeQueryable(branchOf(between))).toBe(false)

    const complete = {
      ...goalLeaf,
      custom_events_goal: {
        ...goalLeaf.custom_events_goal!,
        operator: 'between' as const,
        value_2: 10
      }
    }
    expect(isTreeQueryable(branchOf(complete))).toBe(true)
  })
})

describe('isTreeQueryable — timeframes', () => {
  const withTimeframe = (operator: string, values: string[]): TreeNode =>
    branchOf({
      ...timelineLeaf,
      contact_timeline: {
        ...timelineLeaf.contact_timeline!,
        timeframe_operator: operator as 'anytime',
        timeframe_values: values
      }
    })

  it('ignores leftover values on "anytime"', () => {
    expect(isTreeQueryable(withTimeframe('anytime', ['2026-01-01T00:00:00Z']))).toBe(true)
  })

  it('waits for both ends of a date range', () => {
    expect(isTreeQueryable(withTimeframe('in_date_range', []))).toBe(false)
    expect(isTreeQueryable(withTimeframe('in_date_range', ['2026-01-01T00:00:00Z']))).toBe(false)
    expect(
      isTreeQueryable(withTimeframe('in_date_range', ['2026-01-01T00:00:00Z', '']))
    ).toBe(false)
    expect(
      isTreeQueryable(
        withTimeframe('in_date_range', ['2026-01-01T00:00:00Z', '2026-02-01T00:00:00Z'])
      )
    ).toBe(true)
  })

  it('rejects the two dates left over from a range when the operator became single-date', () => {
    // The query builder wants exactly one value for before_date, so a stale second date is an
    // error rather than something it ignores.
    expect(
      isTreeQueryable(
        withTimeframe('before_date', ['2026-01-01T00:00:00Z', '2026-02-01T00:00:00Z'])
      )
    ).toBe(false)
    expect(isTreeQueryable(withTimeframe('before_date', ['2026-01-01T00:00:00Z']))).toBe(true)
  })

  it('waits for the day count of "in the last"', () => {
    expect(isTreeQueryable(withTimeframe('in_the_last_days', []))).toBe(false)
    expect(isTreeQueryable(withTimeframe('in_the_last_days', ['30']))).toBe(true)
  })
})

describe('isTreeQueryable — filters', () => {
  const withFilter = (filter: Record<string, unknown>): TreeNode =>
    branchOf({
      source: 'contacts',
      contact: { filters: [filter as never] }
    })

  it('waits for the value of a comparison filter', () => {
    expect(
      isTreeQueryable(withFilter({ field_name: 'email', field_type: 'string', operator: 'equals' }))
    ).toBe(false)
    expect(
      isTreeQueryable(
        withFilter({
          field_name: 'email',
          field_type: 'string',
          operator: 'equals',
          string_values: ['a@b.c']
        })
      )
    ).toBe(true)
  })

  it('needs no value for an existence check', () => {
    expect(
      isTreeQueryable(withFilter({ field_name: 'phone', field_type: 'string', operator: 'is_set' }))
    ).toBe(true)
  })

  it('rejects an empty date, which Postgres cannot cast', () => {
    expect(
      isTreeQueryable(
        withFilter({
          field_name: 'created_at',
          field_type: 'time',
          operator: 'before_date',
          string_values: ['']
        })
      )
    ).toBe(false)
  })

  it('needs numbers for a number filter', () => {
    expect(
      isTreeQueryable(
        withFilter({ field_name: 'lifetime_value', field_type: 'number', operator: 'gte' })
      )
    ).toBe(false)
    expect(
      isTreeQueryable(
        withFilter({
          field_name: 'lifetime_value',
          field_type: 'number',
          operator: 'gte',
          number_values: [100]
        })
      )
    ).toBe(true)
  })

  it('needs a json path to read a value out of a json column', () => {
    expect(
      isTreeQueryable(
        withFilter({
          field_name: 'custom_json_1',
          field_type: 'json',
          operator: 'equals',
          string_values: ['pro']
        })
      )
    ).toBe(false)
    expect(
      isTreeQueryable(
        withFilter({
          field_name: 'custom_json_1',
          field_type: 'json',
          operator: 'equals',
          json_path: ['plan'],
          string_values: ['pro']
        })
      )
    ).toBe(true)
  })
})

describe('JSON values read as dates', () => {
  const jsonDate = (operator: string, values: string[]): DimensionFilter =>
    ({
      field_name: 'custom_json_1',
      field_type: 'time',
      json_path: ['renewed_at'],
      operator,
      string_values: values
    }) as DimensionFilter

  it.each([
    ['before_date', ['2026-01-01T00:00:00.000Z']],
    ['after_date', ['2026-01-01T00:00:00.000Z']],
    ['equals', ['2026-01-01T00:00:00.000Z']],
    ['in_the_last_days', ['30']],
    ['not_in_the_last_days', ['30']]
  ])('counts %s with its value', (operator, values) => {
    expect(isFilterComplete(jsonDate(operator, values))).toBe(true)
    expect(isFilterRowComplete(jsonDate(operator, values), true)).toBe(true)
  })

  it('waits for the day count', () => {
    expect(isFilterComplete(jsonDate('in_the_last_days', []))).toBe(false)
  })
})

describe('isFilterRowComplete — what the editor asks for', () => {
  it('wants a path to read a value out of a JSON column', () => {
    const noPath = {
      field_name: 'custom_json_1',
      field_type: 'string',
      operator: 'equals',
      string_values: ['pro']
    } as DimensionFilter
    // The server compiles it against the column itself: kept for the preview and automations…
    expect(isFilterComplete(noPath)).toBe(true)
    // …but the editor's rows never produce it
    expect(isFilterRowComplete(noPath, true)).toBe(false)
    expect(isFilterRowComplete({ ...noPath, json_path: ['plan'] }, true)).toBe(true)
  })

  it('takes a JSON existence check without a path', () => {
    expect(
      isFilterRowComplete({ field_name: 'custom_json_1', field_type: 'string', operator: 'is_set' }, true)
    ).toBe(true)
  })

  it('refuses a blank text value', () => {
    expect(
      isFilterRowComplete(
        { field_name: 'email', field_type: 'string', operator: 'equals', string_values: ['  '] },
        false
      )
    ).toBe(false)
  })

  it('refuses a cleared number, here and in the preview', () => {
    const cleared = {
      field_name: 'custom_number_1',
      field_type: 'number',
      operator: 'gt',
      number_values: [null]
    } as unknown as DimensionFilter
    expect(isFilterComplete(cleared)).toBe(false)
    expect(isFilterRowComplete(cleared, false)).toBe(false)
  })
})

describe('asEditableRoot', () => {
  const leaf: TreeNode = {
    kind: 'leaf',
    leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } }
  }

  it('wraps a condition saved alone at the root in an « all » group', () => {
    expect(asEditableRoot(leaf)).toEqual({ kind: 'branch', branch: { operator: 'and', leaves: [leaf] } })
  })

  it('leaves a group, and nothing, as they are', () => {
    const group: TreeNode = { kind: 'branch', branch: { operator: 'or', leaves: [leaf] } }
    expect(asEditableRoot(group)).toBe(group)
    expect(asEditableRoot(undefined)).toBeUndefined()
  })
})

