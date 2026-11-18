// isSetValue tells whether a block's own value differs from the value it inherits (the type's
// mj-attributes, then the editor's MJML defaults). A new block holds a copy of every default
// (EmailBlockClass.createBlock), so « the key is present » would mark nearly every field: a field
// counts as set only when its value differs once both sides are normalised.

import { normaliseHex } from '../../utils/emailColors'

// Pixel lengths, padding shorthands (1 to 4 values) and colours compare by meaning, not by text.
function normalise(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined
  const text = String(value).trim()
  // '' and 'auto' mean « no value of its own » (a height switched back to Auto)
  if (text === '' || text.toLowerCase() === 'auto') return undefined
  const parts = text.split(/\s+/)
  if (parts.length > 1 && parts.length <= 4 && parts.every((part) => /^-?\d*\.?\d+(px|%|em)?$/i.test(part))) {
    const [top, right = top, bottom = top, left = right] = parts.map(normaliseLength)
    return [top, right, bottom, left].join(' ')
  }
  const hex = normaliseHex(text)
  if (hex) return hex
  return normaliseLength(text)
}

function normaliseLength(part: string): string {
  const zero = /^-?0*\.?0+(px|%|em)?$/i
  if (zero.test(part)) return '0'
  const px = /^(-?\d*\.?\d+)px$/i.exec(part)
  if (px) return `${Number(px[1])}px`
  return part.toLowerCase()
}


export function isSetValue(value: unknown, inherited: unknown): boolean {
  if (value === undefined || value === null) return false
  const own = normalise(value)
  // An empty value is set only when it overrides a value the block would inherit (full width
  // switched off where mj-attributes switch it on); an empty default (href, css-class) is not
  if (own === undefined) return normalise(inherited) !== undefined
  return own !== normalise(inherited)
}
