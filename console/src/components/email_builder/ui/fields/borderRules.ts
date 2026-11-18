// The border rules of the settings kit, shared by BorderField and its tests.

/** One side's border, read from its CSS shorthand. */
export interface BorderValue {
  color?: string
  width?: string
  style?: string
}

/** The four MJML border keys a block writes (or innerBorderTop… mapped by the caller). */
export interface BorderSides {
  borderTop?: string
  borderRight?: string
  borderBottom?: string
  borderLeft?: string
}

export interface BorderPlaceholder {
  color?: string
  width?: string
  style?: string
}

export const isVisibleColor = (color?: string): boolean => Boolean(color) && color !== 'transparent'

export type Side = 'top' | 'right' | 'bottom' | 'left'

export type InternalBorders = Record<Side, BorderValue>

export const SIDES: readonly Side[] = ['top', 'right', 'bottom', 'left']

export const DEFAULT_PLACEHOLDER: BorderPlaceholder = { color: 'transparent', width: '0px', style: 'none' }

/** Reads a CSS border shorthand ("1px solid #000"); none, 0 and unset read as no border. */
export function parseBorderValue(borderStr?: string): BorderValue {
  if (!borderStr || borderStr === 'none') {
    return { width: '0px', style: 'none', color: 'transparent' }
  }
  // Words outside parentheses, so a colour such as rgb(0, 0, 0) stays one word
  const parts = borderStr.trim().match(/(?:[^\s(]+|\([^)]*\))+/g) ?? []
  const styleIndex = parts.findIndex((part) => BORDER_STYLES.has(part.toLowerCase()))
  const style = styleIndex >= 0 ? parts[styleIndex] : undefined
  const rest = parts.filter((_, index) => index !== styleIndex)
  const widthIndex = rest.findIndex((part) => /^(\d*\.?\d+(px|em|rem|%)?|thin|medium|thick)$/i.test(part))
  const width = widthIndex >= 0 ? rest[widthIndex] : undefined
  const color = rest.filter((_, index) => index !== widthIndex).join(' ') || undefined
  return {
    width: width || '0px',
    style: style || 'none',
    color: color || 'transparent'
  }
}

const BORDER_STYLES = new Set(['none', 'hidden', 'solid', 'dashed', 'dotted', 'double', 'groove', 'ridge', 'inset', 'outset'])

/** Writes a border as its CSS shorthand; undefined (no border) for style none or width 0. */
export function formatBorderValue(border?: BorderValue): string | undefined {
  if (!border || border.style === 'none' || border.width === '0px') return undefined
  const width = border.width || '0px'
  const style = border.style || 'none'
  const color = border.color || 'transparent'
  if (style === 'none' || width === '0px') return undefined
  return `${width} ${style} ${color}`
}

/**
 * A colour picked: a visible colour sets width 1px and style solid when they are unset; a cleared
 * or transparent colour resets width and style.
 */
export function borderWithColor(
  border: BorderValue,
  color: string | undefined,
  placeholder: BorderPlaceholder = DEFAULT_PLACEHOLDER
): BorderValue {
  const newColor = color || placeholder.color
  const next = { ...border, color: newColor }
  if (isVisibleColor(newColor) && (!border.width || border.width === '0px')) next.width = '1px'
  if (isVisibleColor(newColor) && (!border.style || border.style === 'none')) next.style = 'solid'
  if (!isVisibleColor(newColor)) {
    next.width = '0px'
    next.style = 'none'
  }
  return next
}

/** A width typed: above 0 it sets style solid when none, and colour #000000 when transparent. */
export function borderWithWidth(border: BorderValue, width: number | null | undefined): BorderValue {
  const next = { ...border, width: width !== null && width !== undefined ? `${width}px` : '0px' }
  if (width && width > 0 && (!border.style || border.style === 'none')) next.style = 'solid'
  if (width && width > 0 && !isVisibleColor(border.color)) next.color = '#000000'
  return next
}

/** A style picked: none resets width to 0; a visible style sets width 1px when 0, and colour #000000 when transparent. */
export function borderWithStyle(
  border: BorderValue,
  style: string | undefined,
  placeholder: BorderPlaceholder = DEFAULT_PLACEHOLDER
): BorderValue {
  const newStyle = style || placeholder.style
  const next = { ...border, style: newStyle }
  if (newStyle === 'none') next.width = '0px'
  if (newStyle !== 'none' && (!border.width || border.width === '0px')) next.width = '1px'
  if (newStyle !== 'none' && !isVisibleColor(border.color)) next.color = '#000000'
  return next
}
