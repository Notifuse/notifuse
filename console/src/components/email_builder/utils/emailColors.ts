import type { EmailBlock } from '../types'

// The attributes whose whole value is a colour.
const COLOR_KEYS = ['color', 'backgroundColor', 'containerBackgroundColor', 'borderColor', 'innerBackgroundColor']

// The border shorthands (« 1px solid #e5e7eb »), whose colour is one of their words.
const SHORTHAND_KEYS = [
  'border',
  'borderTop',
  'borderRight',
  'borderBottom',
  'borderLeft',
  'innerBorderTop',
  'innerBorderRight',
  'innerBorderBottom',
  'innerBorderLeft'
]

const MAX_COLORS = 10

const HEX = /^#([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$/i

// normaliseHex turns a hex colour into lowercase 6 digits (8 when it has an alpha other than ff),
// and anything else (transparent, rgb(), a colour name, a Liquid expression) into undefined.
export function normaliseHex(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const text = value.trim()
  if (!HEX.test(text)) return undefined
  let hex = text.slice(1).toLowerCase()
  if (hex.length === 3) hex = hex.split('').map((c) => c + c).join('')
  if (hex.length === 8 && hex.endsWith('ff')) hex = hex.slice(0, 6)
  return `#${hex}`
}

function blockColors(attributes: Record<string, unknown>): string[] {
  const found: string[] = []
  for (const key of COLOR_KEYS) {
    const hex = normaliseHex(attributes[key])
    if (hex) found.push(hex)
  }
  for (const key of SHORTHAND_KEYS) {
    const value = attributes[key]
    if (typeof value !== 'string') continue
    for (const word of value.trim().split(/\s+/)) {
      const hex = normaliseHex(word)
      if (hex) found.push(hex)
    }
  }
  return found
}

// emailColors lists the colours the email's blocks already use, for the « In this email » row of
// the colour field: most frequent first, then in the order they first appear, at most ten. Only
// block attributes are read; colours written inline in a text's content are not.
export function emailColors(tree: EmailBlock): string[] {
  const counts = new Map<string, number>()
  const visit = (block: { attributes?: Record<string, unknown>; children?: unknown[] }) => {
    if (block.attributes) {
      for (const hex of blockColors(block.attributes)) counts.set(hex, (counts.get(hex) ?? 0) + 1)
    }
    for (const child of block.children ?? []) visit(child as typeof block)
  }
  visit(tree as unknown as { attributes?: Record<string, unknown>; children?: unknown[] })
  // A Map keeps insertion order, and Array.prototype.sort is stable: equal counts keep it.
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, MAX_COLORS)
    .map(([hex]) => hex)
}
