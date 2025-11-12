import { generateHTML, generateJSON } from '@tiptap/react'
import { createBlogExtensions } from '../extensions'
import type { HeadingNode, ContentStats } from './types'

/**
 * Convert Tiptap JSON to HTML string
 */
export const jsonToHtml = (json: any): string => {
  if (!json || !json.type) {
    return ''
  }

  try {
    const html = generateHTML(json, createBlogExtensions())
    return html
  } catch (error) {
    console.error('Error converting JSON to HTML:', error)
    return ''
  }
}

/**
 * Parse HTML string to Tiptap JSON
 */
export const htmlToJson = (html: string): any => {
  if (!html || html.trim() === '') {
    return null
  }

  try {
    const json = generateJSON(html, createBlogExtensions())
    return json
  } catch (error) {
    console.error('Error converting HTML to JSON:', error)
    return null
  }
}

/**
 * Extract plain text content from Tiptap JSON
 * Useful for search indexing and previews
 */
export const extractTextContent = (json: any): string => {
  if (!json) return ''

  const extractText = (node: any): string => {
    if (node.type === 'text') {
      return node.text || ''
    }

    if (node.content && Array.isArray(node.content)) {
      return node.content.map(extractText).join('')
    }

    // Add spacing for block elements
    if (node.type === 'paragraph' || node.type === 'heading') {
      if (node.content) {
        return node.content.map(extractText).join('') + '\n\n'
      }
    }

    if (node.type === 'hardBreak') {
      return '\n'
    }

    return ''
  }

  return extractText(json).trim()
}

/**
 * Extract headings from content to generate table of contents
 */
export const extractHeadings = (json: any): HeadingNode[] => {
  if (!json || !json.content) return []

  const headings: HeadingNode[] = []
  let headingCounter = 0

  const traverse = (node: any) => {
    if (node.type === 'heading' && node.content) {
      const text = node.content
        .filter((n: any) => n.type === 'text')
        .map((n: any) => n.text)
        .join('')

      if (text) {
        headingCounter++
        headings.push({
          id: `heading-${headingCounter}`,
          level: node.attrs?.level || 1,
          text: text,
          children: []
        })
      }
    }

    if (node.content && Array.isArray(node.content)) {
      node.content.forEach(traverse)
    }
  }

  traverse(json)
  return headings
}

/**
 * Count words in content
 */
export const countWords = (json: any): number => {
  const text = extractTextContent(json)
  if (!text) return 0

  // Split by whitespace and filter out empty strings
  const words = text.split(/\s+/).filter((word) => word.length > 0)
  return words.length
}

/**
 * Check if content is empty
 */
export const isEmpty = (json: any): boolean => {
  if (!json) return true

  const text = extractTextContent(json)
  return text.trim().length === 0
}

/**
 * Get content statistics
 */
export const getContentStats = (json: any): ContentStats => {
  const text = extractTextContent(json)
  const words = countWords(json)
  
  // Count paragraphs
  let paragraphs = 0
  const traverse = (node: any) => {
    if (node.type === 'paragraph') {
      paragraphs++
    }
    if (node.content && Array.isArray(node.content)) {
      node.content.forEach(traverse)
    }
  }
  if (json) traverse(json)

  // Calculate reading time (average 200 words per minute)
  const readingTime = Math.ceil(words / 200)

  return {
    characters: text.length,
    words,
    paragraphs,
    readingTime: readingTime || 1
  }
}

