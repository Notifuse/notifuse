import type { EmailBlock } from '../email_builder/types'

/**
 * HTML void elements that must be self-closing in XML
 * @see https://html.spec.whatwg.org/multipage/syntax.html#void-elements
 */
const HTML_VOID_ELEMENTS = [
  'area',
  'base',
  'br',
  'col',
  'embed',
  'hr',
  'img',
  'input',
  'link',
  'meta',
  'param',
  'source',
  'track',
  'wbr'
] as const

/**
 * HTML named entities mapped to their Unicode code points
 * Only entities not predefined in XML (amp, lt, gt, quot, apos) need conversion
 */
const HTML_ENTITY_TO_CODEPOINT: Record<string, number> = {
  // Whitespace and formatting
  nbsp: 160,
  ensp: 8194,
  emsp: 8195,
  thinsp: 8201,

  // Punctuation
  bull: 8226,
  hellip: 8230,
  mdash: 8212,
  ndash: 8211,
  lsquo: 8216,
  rsquo: 8217,
  ldquo: 8220,
  rdquo: 8221,
  laquo: 171,
  raquo: 187,

  // Symbols
  copy: 169,
  reg: 174,
  trade: 8482,
  sect: 167,
  para: 182,
  deg: 176,
  plusmn: 177,
  times: 215,
  divide: 247,
  micro: 181,
  middot: 183,

  // Currency
  euro: 8364,
  pound: 163,
  yen: 165,
  cent: 162,

  // Arrows
  larr: 8592,
  rarr: 8594,
  uarr: 8593,
  darr: 8595,
  harr: 8596,

  // Spanish/French punctuation
  iexcl: 161,
  iquest: 191
}

/**
 * Fixes duplicate attributes on a single tag
 * When an attribute appears multiple times, keeps only the last occurrence
 * @param tagContent - The inner content of an XML/MJML tag
 * @returns Fixed tag content with no duplicate attributes
 */
function fixDuplicateAttributes(tagContent: string): string {
  // Extract all attributes as key-value pairs
  const attributeMap = new Map<string, string>()
  const attributeRegex = /(\S+)="([^"]*)"/g
  let match: RegExpExecArray | null

  // Collect all attributes, later ones overwrite earlier ones
  while ((match = attributeRegex.exec(tagContent)) !== null) {
    const attrName = match[1]
    const attrValue = match[2]
    attributeMap.set(attrName, attrValue)
  }

  // Find the tag name (everything before the first space or the end)
  const tagNameMatch = tagContent.match(/^([^\s>]+)/)
  const tagName = tagNameMatch ? tagNameMatch[1] : ''

  // Reconstruct the tag with unique attributes
  const uniqueAttributes = Array.from(attributeMap.entries())
    .map(([name, value]) => `${name}="${value}"`)
    .join(' ')

  // Return tag name with unique attributes (and preserve any trailing characters like /)
  const hasTrailingSlash = tagContent.trim().endsWith('/')
  return uniqueAttributes
    ? `${tagName} ${uniqueAttributes}${hasTrailingSlash ? ' /' : ''}`
    : `${tagName}${hasTrailingSlash ? ' /' : ''}`
}

/**
 * Preprocesses MJML string to fix common XML issues
 * This makes imports more robust when MJML comes from other editors
 * @param mjmlString - The raw MJML string to preprocess
 * @returns The preprocessed MJML string with fixed XML issues
 */
export function preprocessMjml(mjmlString: string): string {
  let processed = mjmlString

  // Step 1: Convert HTML void tags to self-closing XML format
  // HTML allows <br>, <hr>, <img>, etc. without closing slash
  // XML requires self-closing: <br/>, <hr/>, <img/>
  // This regex matches void tags that are NOT already self-closing
  const voidTagPattern = new RegExp(
    `<(${HTML_VOID_ELEMENTS.join('|')})\\b([^>]*[^/])?>(?!/)`,
    'gi'
  )
  processed = processed.replace(voidTagPattern, (match, tagName, attrs) => {
    // Check if already self-closing (ends with />)
    if (match.endsWith('/>')) {
      return match
    }
    // Convert to self-closing
    const attributes = attrs ? attrs.trimEnd() : ''
    return `<${tagName}${attributes}/>`
  })

  // Step 2: Convert HTML named entities to XML numeric entities
  // XML only predefines: &amp; &lt; &gt; &quot; &apos;
  // HTML entities like &nbsp; must be converted to &#160;
  processed = processed.replace(/&([a-zA-Z]+);/g, (match, entityName) => {
    const lowerName = entityName.toLowerCase()
    // Preserve XML predefined entities
    if (['amp', 'lt', 'gt', 'quot', 'apos'].includes(lowerName)) {
      return match
    }
    // Convert known HTML entities to numeric
    const codePoint = HTML_ENTITY_TO_CODEPOINT[lowerName]
    return codePoint !== undefined ? `&#${codePoint};` : match
  })

  // Step 3: Fix unescaped ampersands in attribute values
  // Use a callback function to process all ampersands within each attribute value
  processed = processed.replace(/="([^"]*)"/g, (_match, attrValue) => {
    // Within this attribute value, escape all unescaped ampersands
    // Don't escape if already part of an entity: &amp;, &lt;, &gt;, &quot;, &apos;, &#123;, &#xAB;
    const fixed = attrValue.replace(/&(?!(amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);)/g, '&amp;')
    return '="' + fixed + '"'
  })

  // Step 4: Fix duplicate attributes in opening tags
  // Match opening tags like <mj-section ...> or <mj-button ... />
  processed = processed.replace(/<([^>]+)>/g, (fullMatch, tagContent) => {
    // Check if this tag has any attributes
    if (!tagContent.includes('=')) {
      return fullMatch // No attributes, return as-is
    }

    // Count attribute occurrences
    const attributes = tagContent.match(/(\S+)="[^"]*"/g) || []
    const attributeNames = attributes.map((attr: string) => attr.split('=')[0])
    const hasDuplicates = new Set(attributeNames).size !== attributeNames.length

    if (hasDuplicates) {
      // Fix the duplicate attributes
      const fixed = fixDuplicateAttributes(tagContent)
      return `<${fixed}>`
    }

    return fullMatch // No duplicates, return as-is
  })

  return processed
}

/**
 * Browser-compatible MJML to JSON converter using DOMParser
 * This is a fallback when mjml2json doesn't work in browser environment
 */
export function convertMjmlToJsonBrowser(mjmlString: string): EmailBlock {
  try {
    // Preprocess MJML to fix common XML issues
    const preprocessedMjml = preprocessMjml(mjmlString)

    // Parse MJML using browser's DOMParser
    const parser = new DOMParser()
    const doc = parser.parseFromString(preprocessedMjml, 'text/xml')

    // Check for parsing errors
    const parserError = doc.querySelector('parsererror')
    if (parserError) {
      throw new Error('Invalid MJML syntax: ' + parserError.textContent)
    }

    // Find the root element (should be mjml)
    const rootElement = doc.documentElement
    if (rootElement.tagName.toLowerCase() !== 'mjml') {
      throw new Error('Root element must be <mjml>')
    }

    // Convert DOM node to EmailBlock format
    return convertDomNodeToEmailBlock(rootElement)
  } catch (error) {
    console.error('Browser MJML to JSON conversion error:', error)
    throw new Error(`Failed to convert MJML to JSON: ${error}`)
  }
}

/**
 * Convert kebab-case to camelCase for React compatibility
 * More comprehensive version that handles all cases
 */
function kebabToCamelCase(str: string): string {
  // Handle special cases first
  if (!str.includes('-')) {
    return str
  }

  // Convert kebab-case to camelCase
  return str.replace(/-([a-zA-Z])/g, (_, letter) => letter.toUpperCase())
}

/**
 * Recursively converts a DOM element to EmailBlock format
 */
function convertDomNodeToEmailBlock(element: Element): EmailBlock {
  // Generate a unique ID for each block
  const generateId = () => Math.random().toString(36).substr(2, 9)

  const block: EmailBlock = {
    id: generateId(),
    type: element.tagName.toLowerCase() as EmailBlock['type'],
    attributes: {}
  }

  // Extract attributes
  if (element.attributes.length > 0) {
    const attributes: Record<string, unknown> = {}
    for (let i = 0; i < element.attributes.length; i++) {
      const attr = element.attributes[i]
      // Convert kebab-case to camelCase for React compatibility
      const attributeName = kebabToCamelCase(attr.name)
      attributes[attributeName] = attr.value
    }
    block.attributes = attributes
  }

  // Special handling for elements that should preserve their inner HTML as content
  // This includes mj-raw, mj-text, mj-button, mj-title, mj-preview
  const contentElements = ['mj-raw', 'mj-text', 'mj-button', 'mj-title', 'mj-preview']
  const tagNameLower = element.tagName.toLowerCase()

  if (contentElements.includes(tagNameLower)) {
    const innerHTML = element.innerHTML
    if (innerHTML.trim()) {
      let content = innerHTML.trim()

      // Special normalization for mj-text: ensure content is wrapped in <p> tags
      // Tiptap editor always wraps content in <p>, so we normalize at import time
      if (tagNameLower === 'mj-text') {
        // Check if content is plain text (not already wrapped in HTML tags)
        const isPlainText = !/^\s*</.test(content)
        if (isPlainText) {
          content = `<p>${content}</p>`
        }
      }

      (block as EmailBlock & { content: string }).content = content
    }
    return block
  }

  // Handle content and children for other elements
  const children: EmailBlock[] = []
  let textContent = ''

  for (let i = 0; i < element.childNodes.length; i++) {
    const child = element.childNodes[i]

    if (child.nodeType === Node.ELEMENT_NODE) {
      // It's an element, recursively convert it
      children.push(convertDomNodeToEmailBlock(child as Element))
    } else if (child.nodeType === Node.TEXT_NODE) {
      // It's text content
      const text = child.textContent?.trim()
      if (text) {
        textContent += text
      }
    }
  }

  // If there are child elements, add them
  if (children.length > 0) {
    // Type assertion is safe here because we're building from parsed MJML
    // The children array will contain the appropriate block types based on the parent
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Dynamic MJML block types require any
    (block as any).children = children
  }

  // If there's text content but no child elements, add it as content
  if (textContent && children.length === 0) {
    (block as EmailBlock & { content: string }).content = textContent
  }

  return block
}
