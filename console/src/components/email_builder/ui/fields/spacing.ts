import type { FieldWrite } from './types'

// The padding rules of the settings kit, shared by SpacingField and the block panels.

export interface SpacingSides {
  top: string | undefined
  right: string | undefined
  bottom: string | undefined
  left: string | undefined
}

export type SpacingSide = keyof SpacingSides

export const SIDES: readonly SpacingSide[] = ['top', 'right', 'bottom', 'left']

export const NO_SIDES: SpacingSides = { top: undefined, right: undefined, bottom: undefined, left: undefined }

export interface SpacingBoxBase {
  /** The box's label, also its accessible name (« Around the text »). */
  label: string
  /** Shown in the centre of the box (« Text »), unless an inner box takes the centre. */
  centerLabel?: string
  /** A nested box in the centre: the button's inner padding inside its outer padding. */
  inner?: SpacingBoxProps
}

/** Per-side mode: the block's paddingTop, paddingRight, paddingBottom, paddingLeft keys. */
export interface SpacingSidesProps extends SpacingBoxBase {
  value: Partial<SpacingSides>
  /** Each side's inherited value, shown as the placeholder of an unset side. */
  inherited?: Partial<SpacingSides>
  /** Writes only the sides edited (one, or the four when linked); map it with toSideAttributes. */
  onChange: FieldWrite<Partial<SpacingSides>>
}

/** Shorthand mode: one MJML padding string such as inner-padding, written as 1, 2 or 4 values. */
export interface SpacingShorthandProps extends SpacingBoxBase {
  value: string | undefined
  /** The inherited shorthand, shown as placeholders and used to fill the sides not typed. */
  inherited: string | undefined
  onChange: FieldWrite<string | undefined>
}

export type SpacingBoxProps = SpacingSidesProps | SpacingShorthandProps

// Detect the mode from the value's runtime type. When `value` is undefined (an mj-button whose
// inner-padding is not set yet) fall back to `inherited`, so shorthand callers, which always pass a
// string inherited value, stay in shorthand mode and write a string. Writing an object there is
// what produced the `map[bottom:0px top:0px]` CSS that Gmail rejects (issue #369).
export const isIndividualPadding = (props: SpacingBoxProps): props is SpacingSidesProps => {
  const probe = props.value !== undefined ? props.value : props.inherited
  return typeof probe === 'object' || probe === undefined
}

/** Reads an MJML padding shorthand (1 to 4 values) into its four sides. */
export function parsePaddingShorthand(padding?: string): SpacingSides {
  if (!padding) return { ...NO_SIDES }
  const parts = padding.trim().split(/\s+/)
  switch (parts.length) {
    case 1:
      return { top: parts[0], right: parts[0], bottom: parts[0], left: parts[0] }
    case 2:
      return { top: parts[0], right: parts[1], bottom: parts[0], left: parts[1] }
    case 3:
      return { top: parts[0], right: parts[1], bottom: parts[2], left: parts[1] }
    case 4:
      return { top: parts[0], right: parts[1], bottom: parts[2], left: parts[3] }
    default:
      return { ...NO_SIDES }
  }
}

/** Writes four sides as the shortest of the 1, 2 or 4-value shorthands; undefined when all are empty. */
export function formatPaddingShorthand(values: SpacingSides): string | undefined {
  const { top, right, bottom, left } = values
  if (!top && !right && !bottom && !left) return undefined
  // Only reached for a side neither typed nor inherited (no inherited value given at all).
  const topVal = top || '0px'
  const rightVal = right || '0px'
  const bottomVal = bottom || '0px'
  const leftVal = left || '0px'
  if (topVal === rightVal && rightVal === bottomVal && bottomVal === leftVal) return topVal
  if (topVal === bottomVal && rightVal === leftVal) return `${topVal} ${rightVal}`
  return `${topVal} ${rightVal} ${bottomVal} ${leftVal}`
}

/**
 * Maps the sides a SpacingField wrote onto the block's keys, only those written:
 * toSideAttributes({ top: '5px' }, 'padding') is { paddingTop: '5px' }.
 */
export function toSideAttributes(sides: Partial<SpacingSides>, prefix: string): Record<string, string | undefined> {
  const out: Record<string, string | undefined> = {}
  SIDES.forEach((side) => {
    if (side in sides) out[`${prefix}${side[0].toUpperCase()}${side.slice(1)}`] = sides[side]
  })
  return out
}
