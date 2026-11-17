// A line height is a multiple of the font size (1.5, written without a unit, the text blocks'
// default), a percentage or pixels.
export type LineHeightUnit = 'x' | '%' | 'px'

/** Splits "1.5", "150%" or "24px" into its number and unit; anything else reads as empty. */
export function parseLineHeight(value?: string): { number?: number; unit: LineHeightUnit } {
  const match = value?.trim().match(/^(\d+(?:\.\d+)?|\.\d+)(px|%)?$/)
  if (!match) return { number: undefined, unit: 'x' }
  return { number: parseFloat(match[1]), unit: (match[2] as '%' | 'px' | undefined) ?? 'x' }
}

// MJML's mj-text default, which the canvas renders when neither the block nor mj-attributes sets a size.
export const DEFAULT_LINE_HEIGHT_FONT_SIZE = 13

/** Rounds to what the field holds: whole pixels and percents, hundredths of a multiple. */
export function roundLineHeight(value: number, unit: LineHeightUnit): number {
  return unit === 'x' ? Math.round(value * 100) / 100 : Math.round(value)
}

/**
 * Re-expresses a line height in another unit with the same rendered height. Pixels depend on the
 * block's font size (in px): 1.5 × 16px is 24px.
 */
export function convertLineHeight(
  value: number,
  from: LineHeightUnit,
  to: LineHeightUnit,
  fontSize: number
): number {
  const size = fontSize > 0 ? fontSize : DEFAULT_LINE_HEIGHT_FONT_SIZE
  const multiple = from === 'x' ? value : from === '%' ? value / 100 : value / size
  const converted = to === 'x' ? multiple : to === '%' ? multiple * 100 : multiple * size
  return roundLineHeight(converted, to)
}
