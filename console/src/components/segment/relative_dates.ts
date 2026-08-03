import { DimensionFilter, TreeNode } from '../../services/api/segment'

// Operators whose meaning moves with the clock, so the segment has to be recomputed daily.
// Keep in sync with filtersHaveRelativeDates / HasRelativeDates in internal/domain/tree.go —
// the backend decides the actual schedule, this only drives the notice shown in the drawer.
const RELATIVE_DATE_OPERATORS = ['in_the_last_days', 'not_in_the_last_days']

const filtersHaveRelativeDates = (filters: DimensionFilter[] | undefined): boolean =>
  (filters ?? []).some((filter: DimensionFilter) =>
    RELATIVE_DATE_OPERATORS.includes(filter.operator as string)
  )

/** Whether a tree contains relative date filters, i.e. whether it is recomputed daily. */
export const treeHasRelativeDates = (tree: TreeNode | null | undefined): boolean => {
  if (!tree) return false

  if (tree.kind === 'branch') {
    // Check all child leaves recursively
    if (tree.branch?.leaves) {
      return tree.branch.leaves.some((leaf: TreeNode) => treeHasRelativeDates(leaf))
    }
    return false
  }

  if (tree.kind === 'leaf') {
    // Check contact timeline conditions for relative date operators
    if (tree.leaf?.contact_timeline) {
      if (tree.leaf.contact_timeline.timeframe_operator === 'in_the_last_days') {
        return true
      }
      if (filtersHaveRelativeDates(tree.leaf.contact_timeline.filters)) {
        return true
      }
    }
    // Check contact property filters for relative date operators
    if (filtersHaveRelativeDates(tree.leaf?.contact?.filters)) {
      return true
    }
    // Check goal conditions for relative date operators
    if (tree.leaf?.custom_events_goal) {
      if (tree.leaf.custom_events_goal.timeframe_operator === 'in_the_last_days') {
        return true
      }
      if (filtersHaveRelativeDates(tree.leaf.custom_events_goal.filters)) {
        return true
      }
    }
    return false
  }

  return false
}
