import { describe, test, expect } from 'vitest'
import {
  EMAIL_AI_TOOLS,
  TOOL_NAMES,
  serializeEmailTree
} from '../email-ai-tools'
import type { EmailBlock } from '../types'

describe('Email AI Tools', () => {
  describe('Tool Definitions', () => {
    test('should have 7 tools defined', () => {
      expect(EMAIL_AI_TOOLS).toHaveLength(7)
    })

    test('should have correct tool names', () => {
      const toolNames = EMAIL_AI_TOOLS.map(tool => tool.name)
      expect(toolNames).toContain('updateBlock')
      expect(toolNames).toContain('addBlock')
      expect(toolNames).toContain('deleteBlock')
      expect(toolNames).toContain('moveBlock')
      expect(toolNames).toContain('selectBlock')
      expect(toolNames).toContain('setEmailTree')
      expect(toolNames).toContain('updateEmailMetadata')
    })

    test('updateBlock tool should have correct schema', () => {
      const tool = EMAIL_AI_TOOLS.find(t => t.name === 'updateBlock')
      expect(tool).toBeDefined()
      expect(tool?.input_schema.type).toBe('object')
      expect(tool?.input_schema.required).toContain('blockId')
      expect(tool?.input_schema.required).toContain('updates')
    })

    test('addBlock tool should have correct enum for blockType', () => {
      const tool = EMAIL_AI_TOOLS.find(t => t.name === 'addBlock')
      expect(tool).toBeDefined()
      const properties = tool?.input_schema.properties as Record<string, { enum?: string[] }>
      expect(properties.blockType.enum).toContain('mj-section')
      expect(properties.blockType.enum).toContain('mj-column')
      expect(properties.blockType.enum).toContain('mj-text')
      expect(properties.blockType.enum).toContain('mj-button')
      expect(properties.blockType.enum).toContain('mj-image')
    })

    test('setEmailTree tool should require tree with specific structure', () => {
      const tool = EMAIL_AI_TOOLS.find(t => t.name === 'setEmailTree')
      expect(tool).toBeDefined()
      expect(tool?.input_schema.required).toContain('tree')
      const properties = tool?.input_schema.properties as Record<string, { properties?: Record<string, { type?: string; enum?: string[] }> }>
      expect(properties.tree.properties?.type.enum).toContain('mjml')
    })
  })

  describe('TOOL_NAMES Constants', () => {
    test('should have all tool names as constants', () => {
      expect(TOOL_NAMES.UPDATE_BLOCK).toBe('updateBlock')
      expect(TOOL_NAMES.ADD_BLOCK).toBe('addBlock')
      expect(TOOL_NAMES.DELETE_BLOCK).toBe('deleteBlock')
      expect(TOOL_NAMES.MOVE_BLOCK).toBe('moveBlock')
      expect(TOOL_NAMES.SELECT_BLOCK).toBe('selectBlock')
      expect(TOOL_NAMES.SET_EMAIL_TREE).toBe('setEmailTree')
      expect(TOOL_NAMES.UPDATE_EMAIL_METADATA).toBe('updateEmailMetadata')
    })
  })

  describe('serializeEmailTree', () => {
    test('should serialize simple tree with type and id', () => {
      const tree: EmailBlock = {
        id: 'root-1',
        type: 'mjml',
        children: []
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('mjml')
      expect(result).toContain('root-1')
    })

    test('should serialize tree with content', () => {
      const tree: EmailBlock = {
        id: 'text-1',
        type: 'mj-text',
        content: '<p>Hello World</p>',
        attributes: {}
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('mj-text')
      expect(result).toContain('text-1')
      expect(result).toContain('Hello World')
    })

    test('should include full content without truncation', () => {
      const longContent = 'A'.repeat(100)
      const tree: EmailBlock = {
        id: 'text-1',
        type: 'mj-text',
        content: `<p>${longContent}</p>`,
        attributes: {}
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain(longContent)
      expect(result).not.toContain('...')
    })

    test('should include key attributes', () => {
      const tree: EmailBlock = {
        id: 'section-1',
        type: 'mj-section',
        attributes: {
          backgroundColor: '#ffffff',
          color: '#333333',
          width: '600px'
        },
        children: []
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('bg: #ffffff')
      expect(result).toContain('color: #333333')
      expect(result).toContain('width: 600px')
    })

    test('should serialize nested children with proper indentation', () => {
      const tree: EmailBlock = {
        id: 'mjml-1',
        type: 'mjml',
        children: [
          {
            id: 'body-1',
            type: 'mj-body',
            children: [
              {
                id: 'section-1',
                type: 'mj-section',
                children: [
                  {
                    id: 'column-1',
                    type: 'mj-column',
                    children: [
                      {
                        id: 'text-1',
                        type: 'mj-text',
                        content: '<p>Nested text</p>',
                        attributes: {}
                      }
                    ]
                  }
                ]
              }
            ]
          }
        ]
      }

      const result = serializeEmailTree(tree)

      // Should contain all block types
      expect(result).toContain('mjml')
      expect(result).toContain('mj-body')
      expect(result).toContain('mj-section')
      expect(result).toContain('mj-column')
      expect(result).toContain('mj-text')

      // Should have proper indentation (spaces)
      const lines = result.split('\n')
      expect(lines.length).toBeGreaterThan(1)
    })

    test('should strip HTML tags from content preview', () => {
      const tree: EmailBlock = {
        id: 'text-1',
        type: 'mj-text',
        content: '<p><strong>Bold</strong> and <em>italic</em> text</p>',
        attributes: {}
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('Bold and italic text')
      expect(result).not.toContain('<strong>')
      expect(result).not.toContain('<em>')
    })

    test('should handle blocks without content or attributes', () => {
      const tree: EmailBlock = {
        id: 'spacer-1',
        type: 'mj-spacer'
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('mj-spacer')
      expect(result).toContain('spacer-1')
    })

    test('should show src attribute as ellipsis for images', () => {
      const tree: EmailBlock = {
        id: 'image-1',
        type: 'mj-image',
        attributes: {
          src: 'https://example.com/very-long-image-path/image.png'
        }
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('src: ...')
    })

    test('should include href for links/buttons', () => {
      const tree: EmailBlock = {
        id: 'button-1',
        type: 'mj-button',
        content: 'Click me',
        attributes: {
          href: 'https://example.com'
        }
      }

      const result = serializeEmailTree(tree)
      expect(result).toContain('href: https://example.com')
    })
  })
})
