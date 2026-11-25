// The column widths of the A/B test table: the counts stay in the panel, the subject takes the rest.
export const VARIANT_WIDTH = 200
/** The subject takes the room the others leave, down to this width. */
const SUBJECT_MIN_WIDTH = 240
export const SENT_WIDTH = 90
export const RATE_WIDTH = 110
/**
 * Preview and the menu of the other actions (icon buttons of 22 px), the web analytics traffic link
 * (31 px: its icon is a child, so it keeps a text button's padding), two gaps of 4 px and the
 * cell's padding.
 */
const ACTIONS_WIDTH = 102
/** The same, with the "Pick as winner" button, in its longest translation. */
const ACTIONS_WIDTH_WITH_PICK = 276

/** The width of the actions column: wider while a winner can be picked. */
export function actionsWidth(winnerSelectable: boolean): number {
  return winnerSelectable ? ACTIONS_WIDTH_WITH_PICK : ACTIONS_WIDTH
}

/** The narrowest the table gets before it scrolls: the fixed columns plus the subject's minimum. */
export function abTableWidth({ sent, winnerSelectable }: { sent: boolean; winnerSelectable: boolean }): number {
  const counts = sent ? SENT_WIDTH + 2 * RATE_WIDTH : 0
  return VARIANT_WIDTH + SUBJECT_MIN_WIDTH + counts + actionsWidth(winnerSelectable)
}
