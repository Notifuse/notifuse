import type { EmailBlock } from '../email_builder/types'

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

  // Fix unescaped ampersands in attribute values
  // Use a callback function to process all ampersands within each attribute value
  processed = processed.replace(/="([^"]*)"/g, (match, attrValue) => {
    // Within this attribute value, escape all unescaped ampersands
    // Don't escape if already part of an entity: &amp;, &lt;, &gt;, &quot;, &apos;, &#123;, &#xAB;
    const fixed = attrValue.replace(/&(?!(amp|lt|gt|quot|apos|#\d+|#x[0-9a-fA-F]+);)/g, '&amp;')
    return '="' + fixed + '"'
  })

  // Fix duplicate attributes in opening tags
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
    type: element.tagName.toLowerCase() as any,
    attributes: {}
  }

  // Extract attributes
  if (element.attributes.length > 0) {
    const attributes: Record<string, any> = {}
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
      ;(block as any).content = innerHTML.trim()
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
    ;(block as any).children = children
  }

  // If there's text content but no child elements, add it as content
  if (textContent && children.length === 0) {
    ;(block as any).content = textContent
  }

  return block
}
