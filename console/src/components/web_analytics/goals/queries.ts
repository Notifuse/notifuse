import { filtersForSchema } from '../lib/dimensions'
import { WebDimensionFilter } from '../lib/types'

export { filtersForSchema }

/** Measures the cards and the drawer chart are built on. */
export const GOAL_MEASURES = ['goals', 'sum_goal_value', 'median_goal_value']

/** Measures the breakdown tables show, as Count / Value columns. */
export const GOAL_BREAKDOWN_MEASURES = ['goals', 'sum_goal_value']

export function goalNameFilter(goalName: string): WebDimensionFilter {
  return { dimension: 'goal_name', operator: 'equals', values: [goalName] }
}

/**
 * Page filters plus the single goal a card or the drawer is about. The goal
 * being viewed is more specific than a page-wide drill-down on the same
 * dimension, so it replaces it rather than ANDing into an empty result.
 */
export function goalFilters(
  filters: WebDimensionFilter[],
  goalName: string
): WebDimensionFilter[] {
  return [
    ...filtersForSchema(filters, 'web_goals').filter((filter) => filter.dimension !== 'goal_name'),
    goalNameFilter(goalName)
  ]
}
