import type { EmailBlock } from '../types'
import { EmailBlockClass } from '../EmailBlockClass'
import { normaliseHex } from './emailColors'

/**
 * The look of each heading level of an email, edited in Default attributes >
 * Headings. It is stored as an mj-style the editor manages, inlined into each
 * heading at send (safe-mjml applies inline mj-style rules to the whole HTML), so
 * every mail client gets it without relying on inheritance.
 *
 * The block's content starts with a comment holding the values as JSON, which the
 * editor reads, followed by the CSS written from them. A block whose CSS no longer
 * matches its JSON was edited by hand and is never overwritten.
 */

export const HEADING_LEVELS = [1, 2, 3, 4, 5, 6] as const
export type HeadingLevel = (typeof HEADING_LEVELS)[number]

export interface HeadingStyle {
  fontFamily?: string
  fontSize: string
  fontWeight: string
  lineHeight: string
  color?: string
  marginTop: string
  marginBottom: string
}

export type HeadingStyles = Record<HeadingLevel, HeadingStyle>

const MARKER = 'notifuse:headings'

const DEFAULT_SIZES: Record<HeadingLevel, string> = {
  1: '28px',
  2: '22px',
  3: '18px',
  4: '16px',
  5: '14px',
  6: '13px'
}

/**
 * Default styles: sizes, weight and spacing only. A heading takes the font and
 * colour of its Text block (white text on a dark section keeps white headings);
 * a level gets its own only when someone sets one. `text` sets them for every level.
 */
export const defaultHeadingStyles = (text: { fontFamily?: string; color?: string } = {}): HeadingStyles =>
  Object.fromEntries(
    HEADING_LEVELS.map((level) => [
      level,
      {
        ...(text.fontFamily ? { fontFamily: text.fontFamily } : {}),
        fontSize: DEFAULT_SIZES[level],
        fontWeight: 'bold',
        lineHeight: '1.3',
        ...(text.color ? { color: text.color } : {}),
        marginTop: '0px',
        marginBottom: '8px'
      }
    ])
  ) as HeadingStyles

// The same object with its keys in one order, so that equal styles serialize alike
const normalize = (styles: HeadingStyles): HeadingStyles =>
  Object.fromEntries(
    HEADING_LEVELS.map((level) => {
      const s = styles[level]
      return [
        level,
        {
          ...(s.fontFamily ? { fontFamily: s.fontFamily } : {}),
          fontSize: s.fontSize,
          fontWeight: s.fontWeight,
          lineHeight: s.lineHeight,
          ...(s.color ? { color: s.color } : {}),
          marginTop: s.marginTop,
          marginBottom: s.marginBottom
        }
      ]
    })
  ) as HeadingStyles

// One CSS rule per level, each selector behind `scope` when given
const cssRules = (styles: HeadingStyles, scope = ''): string =>
  HEADING_LEVELS.map((level) => {
    const s = styles[level]
    const declarations = [
      s.fontFamily && `font-family: ${s.fontFamily}`,
      `font-size: ${s.fontSize}`,
      `font-weight: ${s.fontWeight}`,
      `line-height: ${s.lineHeight}`,
      s.color && `color: ${s.color}`,
      `margin: ${s.marginTop} 0 ${s.marginBottom}`
    ].filter(Boolean)
    return `${scope ? `${scope} ` : ''}h${level} { ${declarations.join('; ')} }`
  }).join('\n')

export const serializeHeadingStyles = (styles: HeadingStyles): string => {
  const normalized = normalize(styles)
  return `/* ${MARKER} ${JSON.stringify(normalized)} */\n${cssRules(normalized)}`
}

/** The heading styles as CSS for the editor canvas, under `scope`. */
export const headingStylesCss = (styles: HeadingStyles, scope: string): string =>
  cssRules(normalize(styles), scope)

const PX = /^(\d{1,3})px$/
const LENGTH_UP_TO_200 = (value: string) => {
  const match = PX.exec(value)
  return !!match && Number(match[1]) <= 200
}
const LINE_HEIGHT = (value: string) => /^\d+(\.\d+)?$/.test(value) || LENGTH_UP_TO_200(value)
const COLOR = (value: string) => /^#[0-9a-f]{3,8}$/i.test(value) || /^rgba?\(\s*[\d.,\s%]+\)$/i.test(value)
const WEIGHT = (value: string) => /^(normal|bold|[1-9]00)$/.test(value)
// Letters, digits, spaces, quotes, commas and hyphens: nothing that ends a rule,
// a comment or a Liquid tag, nor what the MJML converter escapes
const FONT_FAMILY = (value: string) => /^[\p{L}\p{N} ,'"-]+$/u.test(value)

/** The problems of a heading style, empty when it can be written. */
export const validateHeadingStyle = (style: HeadingStyle): string[] => {
  const problems: string[] = []
  if (!LENGTH_UP_TO_200(style.fontSize)) problems.push('fontSize')
  if (!WEIGHT(style.fontWeight)) problems.push('fontWeight')
  if (!LINE_HEIGHT(style.lineHeight)) problems.push('lineHeight')
  if (!LENGTH_UP_TO_200(style.marginTop)) problems.push('marginTop')
  if (!LENGTH_UP_TO_200(style.marginBottom)) problems.push('marginBottom')
  if (style.color !== undefined && !COLOR(style.color)) problems.push('color')
  if (style.fontFamily !== undefined && !FONT_FAMILY(style.fontFamily)) problems.push('fontFamily')
  return problems
}

const contentOf = (block: EmailBlock): string => {
  const content = (block as { content?: unknown }).content
  return typeof content === 'string' ? content : ''
}

const isManagedBlock = (block: EmailBlock) =>
  block.type === 'mj-style' && contentOf(block).trimStart().startsWith(`/* ${MARKER} `)

const headOf = (tree: EmailBlock) => tree.children?.find((child) => child.type === 'mj-head')

/**
 * The email's heading styles: null when it has none, 'hand-edited' when its block
 * no longer matches what the editor wrote (or is no longer inlined).
 */
export const readHeadingStyles = (tree: EmailBlock): HeadingStyles | null | 'hand-edited' => {
  const managed = (headOf(tree)?.children ?? []).filter(isManagedBlock).at(-1)
  if (!managed) return null
  const content = contentOf(managed).trimStart()
  const json = content.slice(`/* ${MARKER} `.length, content.indexOf(' */'))
  let styles: HeadingStyles
  try {
    styles = JSON.parse(json) as HeadingStyles
    if (HEADING_LEVELS.some((level) => !styles[level] || validateHeadingStyle(styles[level]).length > 0)) {
      return 'hand-edited'
    }
  } catch {
    return 'hand-edited'
  }
  const inline = (managed.attributes as Record<string, unknown> | undefined)?.inline
  if (inline !== 'inline' || content !== serializeHeadingStyles(styles)) return 'hand-edited'
  return normalize(styles)
}

const clone = (tree: EmailBlock): EmailBlock => JSON.parse(JSON.stringify(tree)) as EmailBlock

/**
 * The tree with these heading styles: one managed block, placed before the email's
 * other mj-style blocks so that their rules keep the last word. Creates the head and
 * its default attributes when missing, so that the Headings section can be reached.
 */
export const writeHeadingStyles = (tree: EmailBlock, styles: HeadingStyles): EmailBlock => {
  // Edited as plain nodes: the tree types tie each block type to its children's
  const result = clone(tree) as unknown as LooseBlock
  result.children = result.children ?? []
  let head = result.children.find((child) => child.type === 'mj-head')
  if (!head) {
    head = { ...(EmailBlockClass.createBlock('mj-head') as unknown as LooseBlock), children: [] }
    result.children.unshift(head)
  }
  const headChildren = head.children ?? []
  if (!headChildren.some((child) => child.type === 'mj-attributes')) {
    headChildren.unshift({ ...(EmailBlockClass.createBlock('mj-attributes') as unknown as LooseBlock), children: [] })
  }
  const isManaged = (child: LooseBlock) => isManagedBlock(child as unknown as EmailBlock)
  const previous = headChildren.filter(isManaged).at(-1)
  const kept = headChildren.filter((child) => !isManaged(child))
  const managed: LooseBlock = {
    id: previous?.id ?? EmailBlockClass.generateId(),
    type: 'mj-style',
    attributes: { inline: 'inline' },
    content: serializeHeadingStyles(styles)
  }
  const firstStyle = kept.findIndex((child) => child.type === 'mj-style')
  kept.splice(firstStyle === -1 ? kept.length : firstStyle, 0, managed)
  head.children = kept
  return result as unknown as EmailBlock
}

interface LooseBlock {
  id: string
  type: string
  attributes?: Record<string, unknown>
  content?: string
  children?: LooseBlock[]
}

const HEADING_TAG = /<h[1-6][\s>/]/i

const hasTextHeading = (tree: EmailBlock): boolean => {
  const visit = (block: EmailBlock): boolean =>
    (block.type === 'mj-text' && HEADING_TAG.test(contentOf(block))) ||
    (block.children ?? []).some(visit)
  const body = tree.children?.find((child) => child.type === 'mj-body')
  return !!body && visit(body)
}

/**
 * Adds the default heading styles when an edit makes the email's first Text-block
 * heading. An email that already had headings keeps its look until someone edits
 * Headings, and an email with styles keeps them.
 */
export const ensureHeadingStyles = (before: EmailBlock, after: EmailBlock): EmailBlock => {
  if (hasTextHeading(before) || !hasTextHeading(after) || readHeadingStyles(after) !== null) return after
  return writeHeadingStyles(after, defaultHeadingStyles())
}

/** The colour a heading level keeps from the colour picker: none once it is cleared. */
export const colorFromPicker = (value: string | undefined): string | undefined =>
  value && value !== 'transparent' ? value : undefined

export const HEADING_FIELDS = [
  'fontFamily',
  'fontSize',
  'fontWeight',
  'lineHeight',
  'color',
  'marginTop',
  'marginBottom'
] as const satisfies readonly (keyof HeadingStyle)[]
export type HeadingField = (typeof HEADING_FIELDS)[number]

// A value as compared with the default: colours by their hex, a zero length with or without px
const comparable = (field: HeadingField, value: string | undefined): string | undefined => {
  const text = value?.trim()
  if (!text) return undefined
  if (field === 'color') return normaliseHex(text) ?? text.toLowerCase()
  // A number or a length in pixels, by its value: 0 and 0px, 1.30 and 1.3 are the same
  const number = /^(-?\d*\.?\d+)(px)?$/i.exec(text)
  if (number) return `${Number(number[1])}${number[2] || Number(number[1]) === 0 ? 'px' : ''}`
  return text.toLowerCase()
}

/** The level's own value differs from the default one: the field shows a « set » dot. */
export const headingFieldDiffers = (style: HeadingStyle, base: HeadingStyle, field: HeadingField): boolean =>
  comparable(field, style[field]) !== comparable(field, base[field])

/** The level has a field that differs from the defaults. */
export const headingLevelDiffers = (styles: HeadingStyles, defaults: HeadingStyles, level: HeadingLevel): boolean =>
  HEADING_FIELDS.some((field) => headingFieldDiffers(styles[level], defaults[level], field))

/** How many heading fields, all levels together, differ from the defaults (0 for hand-edited styles). */
export const headingSetCount = (tree: EmailBlock): number => {
  const styles = readHeadingStyles(tree)
  if (!styles || styles === 'hand-edited') return 0
  const defaults = defaultHeadingStyles()
  return HEADING_LEVELS.reduce(
    (count, level) =>
      count + HEADING_FIELDS.filter((field) => headingFieldDiffers(styles[level], defaults[level], field)).length,
    0
  )
}
