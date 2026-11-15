// The column widths of the A/B test table: the counts stay in the panel, the subject takes the rest.
export const VARIANT_WIDTH = 200
/** The subject takes the room the others leave, down to this width. */
const SUBJECT_MIN_WIDTH = 240
export const SENT_WIDTH = 90
export const RATE_WIDTH = 110
/**
 * Preview and test send (icon buttons of 22 px), two web analytics links (31 px: their icon is a
 * child, so they keep a text button's padding), three gaps of 4 px and the cell's padding: measured
 * at 134 px in the console.
 */
const ACTIONS_WIDTH = 136
/** The same, with the "Pick as winner" button, in its longest translation. */
const ACTIONS_WIDTH_WITH_PICK = 310

/** The width of the actions column: wider while a winner can be picked. */
export function actionsWidth(winnerSelectable: boolean): number {
  return winnerSelectable ? ACTIONS_WIDTH_WITH_PICK : ACTIONS_WIDTH
}

/** The narrowest the table gets before it scrolls: the fixed columns plus the subject's minimum. */
export function abTableWidth({ sent, winnerSelectable }: { sent: boolean; winnerSelectable: boolean }): number {
  const counts = sent ? SENT_WIDTH + 2 * RATE_WIDTH : 0
  return VARIANT_WIDTH + SUBJECT_MIN_WIDTH + counts + actionsWidth(winnerSelectable)
}
