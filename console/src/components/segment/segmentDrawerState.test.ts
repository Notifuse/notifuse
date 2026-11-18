import { describe, it, expect } from 'vitest'
import type { TreeNode } from '../../services/api/segment'
import { hasEmptyGroup, saveBlockers, segmentFormSnapshot, treeShape } from './segmentDrawerState'

const leaf: TreeNode = {
  kind: 'leaf',
  leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: 'news', status: 'active' } }
}
const tree = (...leaves: TreeNode[]): TreeNode => ({ kind: 'branch', branch: { operator: 'and', leaves } })

describe('saveBlockers', () => {
  const ready = { tree: tree(leaf), name: 'Big spenders', editingOpen: false, isEdit: false, idCheck: 'available' as const }

  it('lets a complete new segment be created', () => {
    expect(saveBlockers(ready)).toEqual([])
  })

  it('lists every reason, in the order the summary shows them', () => {
    expect(saveBlockers({ ...ready, tree: tree(), editingOpen: true, name: '' })).toEqual([
      'condition_open',
      'no_condition',
      'name_missing'
    ])
  })

  it('refuses a nested group left empty, which the server rejects', () => {
    expect(saveBlockers({ ...ready, tree: tree(leaf, tree()) })).toEqual(['empty_group'])
    expect(saveBlockers({ ...ready, tree: tree(leaf, tree(leaf, tree())) })).toEqual(['empty_group'])
    // An empty root is a segment without condition, said once
    expect(saveBlockers({ ...ready, tree: tree() })).toEqual(['no_condition'])
  })

  it('waits for the ID check, and refuses a taken or empty ID, on create only', () => {
    expect(saveBlockers({ ...ready, idCheck: 'checking' })).toEqual(['id_checking'])
    // A name typed but not checked yet: the check starts after the debounce
    expect(saveBlockers({ ...ready, idCheck: 'idle' })).toEqual(['id_checking'])
    expect(saveBlockers({ ...ready, idCheck: 'taken' })).toEqual(['id_taken'])
    expect(saveBlockers({ ...ready, name: '会員' })).toEqual(['id_empty'])
    // A failed check does not block: the server refuses a duplicate anyway
    expect(saveBlockers({ ...ready, idCheck: 'failed' })).toEqual([])
  })

  it('ignores the ID when editing: a saved segment keeps its own', () => {
    expect(saveBlockers({ ...ready, isEdit: true, name: '会員', idCheck: 'taken' })).toEqual([])
  })
})

describe('hasEmptyGroup', () => {
  it('finds an empty group at any depth, never the root', () => {
    expect(hasEmptyGroup(tree(leaf, tree(leaf)))).toBe(false)
    expect(hasEmptyGroup(tree(leaf, tree(tree(leaf), tree())))).toBe(true)
    expect(hasEmptyGroup(tree())).toBe(false)
    expect(hasEmptyGroup(undefined)).toBe(false)
  })
})

describe('treeShape', () => {
  it('counts conditions and nested groups', () => {
    expect(treeShape(tree(leaf, tree(leaf, leaf), tree()))).toEqual({ conditions: 3, groups: 2 })
    expect(treeShape(undefined)).toEqual({ conditions: 0, groups: 0 })
  })

  it('leaves out a condition just added and still blank', () => {
    const blank: TreeNode = { kind: 'leaf', leaf: { source: 'contact_lists', contact_list: { operator: 'in', list_id: '' } } }
    expect(treeShape(tree(leaf, blank))).toEqual({ conditions: 1, groups: 0 })
  })
})

describe('segmentFormSnapshot', () => {
  it('reads a condition rebuilt in another key order as unchanged', () => {
    const reordered: TreeNode = {
      leaf: { contact_list: { status: 'active', list_id: 'news', operator: 'in' }, source: 'contact_lists' },
      kind: 'leaf'
    }
    expect(segmentFormSnapshot({ name: 'a', tree: tree(leaf) })).toBe(segmentFormSnapshot({ name: 'a', tree: tree(reordered) }))
  })

  it('sees a changed name, colour, timezone or tree', () => {
    const base = { name: 'a', color: 'blue', timezone: 'UTC', tree: tree(leaf) }
    const snapshot = segmentFormSnapshot(base)
    expect(segmentFormSnapshot({ ...base, name: 'b' })).not.toBe(snapshot)
    expect(segmentFormSnapshot({ ...base, color: 'red' })).not.toBe(snapshot)
    expect(segmentFormSnapshot({ ...base, timezone: 'Europe/Paris' })).not.toBe(snapshot)
    expect(segmentFormSnapshot({ ...base, tree: tree() })).not.toBe(snapshot)
  })
})
