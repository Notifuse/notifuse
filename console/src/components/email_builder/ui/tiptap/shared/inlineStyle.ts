/**
 * Reads and edits an HTML `style` attribute as written, without the browser's CSS
 * parser. The CSSOM drops what it cannot parse and rewrites what it keeps: a Liquid
 * value (`color: {{ brand_color }}`) vanishes and `#ff0000` comes back as
 * `rgb(255, 0, 0)`. Email styles are saved and sent as typed, so the editor keeps
 * every declaration verbatim and only rewrites the property it is asked to change.
 */

interface Declaration {
  // Lower-case property name, or null for a part it cannot read as one
  // (a Liquid tag around declarations, a stray word): such parts are kept as is.
  property: string | null
  raw: string
}

/**
 * Splits on the semicolons that end declarations: not those inside quotes,
 * parentheses (`url(data:…;base64,…)`) or Liquid markup.
 */
// Liquid tags that open and close a block, which may hold whole declarations
const LIQUID_BLOCK_TAGS = ['if', 'unless', 'case', 'for', 'capture', 'raw', 'comment', 'tablerow']

const liquidTagName = (tag: string): string => /^\{%-?\s*(\w+)/.exec(tag)?.[1]?.toLowerCase() ?? ''

const splitDeclarations = (style: string): string[] => {
  const parts: string[] = []
  let current = ''
  let quote: string | null = null
  let parens = 0
  let liquidCloser: string | null = null
  let liquidTag = ''
  // Open Liquid blocks: a semicolon inside one ends no declaration
  let blocks = 0

  for (let i = 0; i < style.length; i++) {
    const char = style[i]
    const pair = style.slice(i, i + 2)

    if (liquidCloser) {
      if (pair === liquidCloser) {
        current += pair
        liquidTag += pair
        i++
        if (liquidCloser === '%}') {
          const name = liquidTagName(liquidTag)
          if (LIQUID_BLOCK_TAGS.includes(name)) blocks++
          else if (name.startsWith('end') && LIQUID_BLOCK_TAGS.includes(name.slice(3))) blocks = Math.max(0, blocks - 1)
        }
        liquidCloser = null
        continue
      }
      current += char
      liquidTag += char
      continue
    }
    if (pair === '{{' || pair === '{%') {
      liquidCloser = pair === '{{' ? '}}' : '%}'
      liquidTag = pair
      current += pair
      i++
      continue
    }
    if (quote) {
      if (char === quote) quote = null
      current += char
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
    } else if (char === '(') {
      parens++
    } else if (char === ')') {
      parens = Math.max(0, parens - 1)
    } else if (char === ';' && parens === 0 && blocks === 0) {
      parts.push(current)
      current = ''
      continue
    }
    current += char
  }
  parts.push(current)
  return parts.map((part) => part.trim()).filter(Boolean)
}

const PROPERTY_NAME = /^-{0,2}[a-zA-Z][a-zA-Z0-9-]*$/

// A Liquid block ({% if %}…{% endif %}) may hold several declarations: it is one
// part, kept whole, that edits never cut or read into
const parseDeclarations = (style: string | null | undefined): Declaration[] =>
  splitDeclarations(style ?? '').map((raw) => {
    const colon = raw.indexOf(':')
    const name = colon > 0 && !raw.includes('{%') ? raw.slice(0, colon).trim() : ''
    return { property: PROPERTY_NAME.test(name) ? name.toLowerCase() : null, raw }
  })

/** The Liquid blocks of a style, kept whole, or null. */
export const liquidBlocksOf = (style: string | null | undefined): string | null =>
  serialize(parseDeclarations(style).filter((d) => d.property === null && d.raw.includes('{%')))

const serialize = (declarations: Declaration[]): string | null =>
  declarations.length > 0 ? declarations.map((d) => d.raw).join('; ') : null

/** Normalises a style attribute's spacing; null when it holds nothing. */
export const normalizeStyle = (style: string | null | undefined): string | null =>
  serialize(parseDeclarations(style))

/** The value of the last declaration of `property`, as written, or '' when absent. */
export const getStyleProperty = (style: string | null | undefined, property: string): string => {
  const name = property.toLowerCase()
  const match = parseDeclarations(style)
    .filter((d) => d.property === name)
    .pop()
  if (!match) return ''
  return match.raw.slice(match.raw.indexOf(':') + 1).trim()
}

/**
 * Sets `property` to `value`, or removes it when `value` is empty. The first
 * declaration of the property is replaced in place and any later one removed; the
 * other declarations are left as written. Returns null when nothing remains.
 */
export const setStyleProperty = (
  style: string | null | undefined,
  property: string,
  value: string | null | undefined
): string | null => {
  const name = property.toLowerCase()
  const trimmed = (value ?? '').trim()
  const result: Declaration[] = []
  let placed = false

  for (const declaration of parseDeclarations(style)) {
    if (declaration.property !== name) {
      result.push(declaration)
      continue
    }
    if (!placed && trimmed) {
      result.push({ property: name, raw: `${name}: ${trimmed}` })
    }
    placed = true
  }
  if (!placed && trimmed) {
    result.push({ property: name, raw: `${name}: ${trimmed}` })
  }
  return serialize(result)
}

/** Keeps only the declarations whose property is in `properties`. */
export const pickStyleProperties = (
  style: string | null | undefined,
  properties: readonly string[]
): string | null =>
  serialize(
    parseDeclarations(style).filter((d) => d.property !== null && properties.includes(d.property))
  )

/** Adds the declarations of `extra` over `base`, property by property. */
export const mergeStyles = (
  base: string | null | undefined,
  extra: string | null | undefined
): string | null => {
  let merged = normalizeStyle(base)
  for (const declaration of parseDeclarations(extra)) {
    if (declaration.property === null) {
      merged = serialize([...parseDeclarations(merged), declaration])
    } else {
      merged = setStyleProperty(
        merged,
        declaration.property,
        declaration.raw.slice(declaration.raw.indexOf(':') + 1)
      )
    }
  }
  return merged
}

/**
 * ProseMirror writes a style attribute through the CSSOM (`dom.style.cssText`),
 * which rewrites it the way normalizeStyle avoids. Each style is therefore also
 * rendered in this attribute, which getEditorHTML puts back as the style as written
 * and removes. Pasting the editor's own HTML reads it first for the same reason.
 */
export const RAW_STYLE_ATTRIBUTE = 'data-raw-style'

/** The attributes that render `style`, nothing when it is empty. */
export const styleAttributes = (style: unknown): Record<string, string> =>
  typeof style === 'string' && style ? { style, [RAW_STYLE_ATTRIBUTE]: style } : {}

/** The style of an element as written, from the editor's own copy when it has one. */
export const readStyleAttribute = (element: HTMLElement): string | null =>
  normalizeStyle(element.getAttribute(RAW_STYLE_ATTRIBUTE) ?? element.getAttribute('style'))

/** Drops the declarations whose property is in `properties`. */
export const omitStyleProperties = (
  style: string | null | undefined,
  properties: readonly string[]
): string | null =>
  serialize(
    parseDeclarations(style).filter((d) => d.property === null || !properties.includes(d.property))
  )

/** The properties a style declares, lower-cased, in order. */
export const stylePropertyNames = (style: string | null | undefined): string[] =>
  parseDeclarations(style).flatMap((d) => (d.property ? [d.property] : []))

// data-* attributes the editors use for themselves, never kept as the author's
const EDITOR_DATA_ATTRIBUTES = new Set([RAW_STYLE_ATTRIBUTE, 'data-inline-doc', 'data-pm-slice'])

/** The author's data-* attributes of an element, or null when it has none. */
export const dataAttributesOf = (element: HTMLElement): Record<string, string> | null => {
  const found = Array.from(element.attributes).filter(
    (attribute) => attribute.name.startsWith('data-') && !EDITOR_DATA_ATTRIBUTES.has(attribute.name)
  )
  return found.length > 0 ? Object.fromEntries(found.map((attribute) => [attribute.name, attribute.value])) : null
}

