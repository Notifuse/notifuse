import type { TreeNode } from '../../services/api/segment'
import { HasLeaf, isLeafComplete } from './tree_completeness'
import { segmentIdProblem } from './segmentId'

/**
 * Where the check of a new segment's ID stands: not started (no name), waiting out its debounce
 * or in flight, free, taken, or failed (the server could not say). A failed check is not read as
 * free: the summary says so, and the server refuses a duplicate anyway.
 */
export type SegmentIdCheck = 'idle' | 'checking' | 'available' | 'taken' | 'failed'

export type SaveBlocker =
  | 'condition_open'
  | 'no_condition'
  | 'empty_group'
  | 'name_missing'
  | 'id_empty'
  | 'id_checking'
  | 'id_taken'

export interface SaveBlockersInput {
  tree?: TreeNode
  name: string
  /** A condition is open in its editor: its edits are not in the tree yet. */
  editingOpen: boolean
  /** Editing a saved segment: its ID is fixed, whatever the name says. */
  isEdit: boolean
  idCheck: SegmentIdCheck
}

/** Why the segment cannot be saved yet, in the order the summary lists them. Empty: it can. */
export function saveBlockers({ tree, name, editingOpen, isEdit, idCheck }: SaveBlockersInput): SaveBlocker[] {
  const blockers: SaveBlocker[] = []
  if (editingOpen) blockers.push('condition_open')
  if (!HasLeaf(tree)) blockers.push('no_condition')
  else if (hasEmptyGroup(tree)) blockers.push('empty_group')
  if (!name.trim()) {
    blockers.push('name_missing')
    return blockers
  }
  if (isEdit) return blockers
  if (segmentIdProblem(name)) blockers.push('id_empty')
  // Not checked yet (the check starts after the typing) is as good as on its way
  else if (idCheck === 'checking' || idCheck === 'idle') blockers.push('id_checking')
  else if (idCheck === 'taken') blockers.push('id_taken')
  return blockers
}

/**
 * Whether a nested group holds no condition and no group: the server refuses a branch without
 * leaves. The root is left out, as an empty root is a segment without condition.
 */
export function hasEmptyGroup(tree?: TreeNode): boolean {
  const walk = (node: TreeNode, isRoot: boolean): boolean => {
    if (node.kind !== 'branch') return false
    const leaves = node.branch?.leaves ?? []
    if (!isRoot && leaves.length === 0) return true
    return leaves.some((child) => walk(child, false))
  }
  return tree ? walk(tree, true) : false
}

/**
 * The number of conditions and of nested groups of a tree, for the summary. A condition still
 * missing what the server needs (one just added, being filled in) is not counted yet.
 */
export function treeShape(tree?: TreeNode): { conditions: number; groups: number } {
  let conditions = 0
  let groups = 0
  const walk = (node: TreeNode, isRoot: boolean) => {
    if (node.kind === 'leaf') {
      if (isLeafComplete(node.leaf)) conditions++
      return
    }
    if (!isRoot) groups++
    for (const child of node.branch?.leaves ?? []) walk(child, false)
  }
  if (tree) walk(tree, true)
  return { conditions, groups }
}

// JSON with object keys sorted, so that a condition confirmed unchanged (its form rebuilds the
// object in another key order) still reads as the same value.
function stableStringify(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${stableStringify(item)}`).join(',')}}`
  }
  return JSON.stringify(value ?? null)
}

/** What the drawer compares to tell whether there is something to lose on closing. */
export function segmentFormSnapshot(values: {
  name?: string
  color?: string
  timezone?: string
  tree?: TreeNode
}): string {
  return stableStringify([values.name ?? '', values.color ?? '', values.timezone ?? '', values.tree ?? null])
}
