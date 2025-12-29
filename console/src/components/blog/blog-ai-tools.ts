import type { LLMTool } from '../../services/api/llm'

export interface BlogMetadata {
  title?: string
  excerpt?: string
  meta_title?: string
  meta_description?: string
  keywords?: string[]
  og_title?: string
  og_description?: string
}

export const BLOG_TOOL_NAMES = {
  UPDATE_CONTENT: 'update_blog_content',
  UPDATE_METADATA: 'update_blog_metadata'
} as const

// Tool definition for updating blog content
export const UPDATE_BLOG_CONTENT_TOOL: LLMTool = {
  name: 'update_blog_content',
  description:
    'Update the blog post content in the editor. Use this when you have generated or modified content for the user.',
  input_schema: {
    type: 'object',
    properties: {
      content: {
        type: 'object',
        description:
          'Tiptap JSON document with type "doc" and content array containing heading, paragraph, bulletList, etc.'
      },
      message: {
        type: 'string',
        description: 'Brief message to show the user about what was updated'
      }
    },
    required: ['content', 'message']
  }
}

// Tool definition for updating blog metadata (title, excerpt, SEO, Open Graph)
export const UPDATE_BLOG_METADATA_TOOL: LLMTool = {
  name: 'update_blog_metadata',
  description:
    'Update the blog post metadata including title, excerpt, SEO settings (meta title, meta description, keywords), and Open Graph settings (og_title, og_description). Use this when asked to generate or update titles, descriptions, SEO content, or social sharing metadata. Only include fields you want to update.',
  input_schema: {
    type: 'object',
    properties: {
      title: {
        type: 'string',
        description: 'The blog post title (max 500 characters)'
      },
      excerpt: {
        type: 'string',
        description: 'Brief summary shown in post listings and previews (max 500 characters)'
      },
      meta_title: {
        type: 'string',
        description: 'SEO meta title for search engines (recommended 50-60 characters)'
      },
      meta_description: {
        type: 'string',
        description: 'SEO meta description for search results (recommended 150-160 characters)'
      },
      keywords: {
        type: 'array',
        items: { type: 'string' },
        description: 'SEO keywords as an array of strings'
      },
      og_title: {
        type: 'string',
        description: 'Open Graph title for social media sharing (max 60 characters)'
      },
      og_description: {
        type: 'string',
        description: 'Open Graph description for social media sharing (max 160 characters)'
      },
      message: {
        type: 'string',
        description: 'Brief message to show the user about what was updated'
      }
    },
    required: ['message']
  }
}

export const BLOG_AI_TOOLS: LLMTool[] = [UPDATE_BLOG_CONTENT_TOOL, UPDATE_BLOG_METADATA_TOOL]

// Helper to extract plain text from Tiptap JSON
export function extractTextFromTiptap(doc: Record<string, unknown>): string {
  const extractFromNode = (node: Record<string, unknown>): string => {
    if (node.type === 'text' && typeof node.text === 'string') {
      return node.text
    }
    if (Array.isArray(node.content)) {
      return node.content
        .map((child) => extractFromNode(child as Record<string, unknown>))
        .join('')
    }
    return ''
  }

  const processBlock = (node: Record<string, unknown>): string => {
    const text = extractFromNode(node)
    // Add newlines after block elements
    if (['paragraph', 'heading', 'listItem', 'blockquote'].includes(node.type as string)) {
      return text + '\n'
    }
    if (node.type === 'bulletList' || node.type === 'orderedList') {
      return (
        (Array.isArray(node.content)
          ? node.content
              .map((item) => '• ' + extractFromNode(item as Record<string, unknown>))
              .join('\n')
          : '') + '\n'
      )
    }
    return text
  }

  if (doc.type === 'doc' && Array.isArray(doc.content)) {
    return doc.content
      .map((node) => processBlock(node as Record<string, unknown>))
      .join('')
      .trim()
  }
  return ''
}
