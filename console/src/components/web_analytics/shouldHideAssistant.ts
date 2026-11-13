import { type WebAnalyticsTab } from './context'

/**
 * Two tabs configure rather than read, and the assistant is hidden on both.
 *
 * The filters tab configures attribution rewrite rules rather than reading a
 * report: its gate runs in config mode, the period picker and filter bar are not
 * on the page at all, and "filter" there means a snake_case attribution rule, not
 * a camelCase query filter. Every tool the assistant owns would mutate state the
 * operator cannot see.
 *
 * The annotations tab is a CRUD list over rows the assistant can neither read nor
 * write: it has no period, no filter bar and no report, so the same argument holds
 * - and a panel floating over a table of edit buttons only covers them up.
 *
 * Both tabs are excluded from NAVIGABLE_TABS (web-analytics-ai-tools.ts), so
 * navigate_to_tab cannot send the operator to a place the panel is invisible.
 * The two rules are written independently and cross-checked by a test rather than
 * defined in terms of each other, so the check has something to catch.
 */
export function shouldHideAssistant(tab: WebAnalyticsTab): boolean {
  return tab === 'filters' || tab === 'annotations'
}
