/** The Broadcast column takes the room the others leave, down to this width. */
export const BROADCAST_NAME_MIN_WIDTH = 200

/**
 * The narrowest the table gets before it scrolls: the fixed widths of its columns
 * plus the minimum of the one that takes the rest. Only widths in pixels count.
 */
export function minTableWidth(columns: { width?: string | number }[], nameMinWidth: number): number {
  return columns.reduce((sum, c) => sum + (typeof c.width === 'number' ? c.width : 0), nameMinWidth)
}
