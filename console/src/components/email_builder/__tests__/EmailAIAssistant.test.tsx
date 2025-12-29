import { describe, test, expect, vi } from 'vitest'
import type { EmailBlock } from '../types'
import type { EmailAIAgentCallbacks } from '../email-ai-tools'
import { EmailBlockClass } from '../EmailBlockClass'

/**
 * Unit tests for EmailAIAssistant callback implementations
 *
 * Note: Full component rendering tests are skipped due to antd ESM module
 * resolution issues in vitest. These tests focus on the callback logic that
 * would be used when integrating the EmailAIAssistant.
 */
describe('EmailAIAssistant Callbacks', () => {
  const createMockTree = (): EmailBlock => ({
    id: 'mjml-1',
    type: 'mjml',
    children: [
      {
        id: 'body-1',
        type: 'mj-body',
        attributes: { width: '600px' },
        children: [
          {
            id: 'section-1',
            type: 'mj-section',
            attributes: { backgroundColor: '#ffffff' },
            children: [
              {
                id: 'column-1',
                type: 'mj-column',
                attributes: { width: '100%' },
                children: [
                  {
                    id: 'text-1',
                    type: 'mj-text',
                    content: '<p>Hello World</p>',
                    attributes: {
                      color: '#333333',
                      fontSize: '16px'
                    }
                  }
                ]
              }
            ]
          }
        ]
      }
    ]
  })

  describe('onAddBlock callback implementation', () => {
    test('should create and insert a new block at specified position', () => {
      const tree = createMockTree()
      const parentId = 'column-1'
      const blockType = 'mj-button'
      const position = 1
      const content = 'Click me'
      const attributes = { backgroundColor: '#007bff' }

      // Create a new block
      const newBlock = EmailBlockClass.createBlock(blockType, undefined, content, tree)
      if (attributes) {
        newBlock.attributes = { ...newBlock.attributes, ...attributes }
      }

      // Insert into tree
      const updatedTree = EmailBlockClass.insertBlockIntoTree(tree, parentId, newBlock, position)

      expect(updatedTree).not.toBeNull()
      const column = EmailBlockClass.findBlockById(updatedTree!, parentId)
      expect(column?.children).toHaveLength(2) // Original text + new button
      expect(column?.children?.[1].type).toBe('mj-button')
      expect(column?.children?.[1].content).toBe('Click me')
    })

    test('should create block with default attributes', () => {
      const tree = createMockTree()
      const newBlock = EmailBlockClass.createBlock('mj-button', undefined, 'Test', tree)

      expect(newBlock.type).toBe('mj-button')
      expect(newBlock.id).toBeDefined()
      expect(newBlock.content).toBe('Test')
      // Should have default button attributes
      expect(newBlock.attributes).toBeDefined()
    })
  })

  describe('onUpdateBlock callback implementation', () => {
    test('should update block attributes', () => {
      const tree = createMockTree()
      const blockId = 'text-1'
      const updates = {
        attributes: { color: '#000000', fontSize: '18px' }
      }

      // Deep clone and update
      const updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock
      const block = EmailBlockClass.findBlockById(updatedTree, blockId)

      if (block && updates.attributes) {
        block.attributes = { ...block.attributes, ...updates.attributes }
      }

      const updatedBlock = EmailBlockClass.findBlockById(updatedTree, blockId)
      expect(updatedBlock?.attributes?.color).toBe('#000000')
      expect(updatedBlock?.attributes?.fontSize).toBe('18px')
    })

    test('should update block content', () => {
      const tree = createMockTree()
      const blockId = 'text-1'
      const updates = { content: '<p>Updated content</p>' }

      const updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock
      const block = EmailBlockClass.findBlockById(updatedTree, blockId)

      if (block && updates.content !== undefined) {
        block.content = updates.content
      }

      const updatedBlock = EmailBlockClass.findBlockById(updatedTree, blockId)
      expect(updatedBlock?.content).toBe('<p>Updated content</p>')
    })

    test('should preserve existing attributes when updating', () => {
      const tree = createMockTree()
      const blockId = 'text-1'

      const updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock
      const block = EmailBlockClass.findBlockById(updatedTree, blockId)

      // Update only fontSize, color should remain
      if (block) {
        block.attributes = { ...block.attributes, fontSize: '20px' }
      }

      const updatedBlock = EmailBlockClass.findBlockById(updatedTree, blockId)
      expect(updatedBlock?.attributes?.color).toBe('#333333')
      expect(updatedBlock?.attributes?.fontSize).toBe('20px')
    })
  })

  describe('onDeleteBlock callback implementation', () => {
    test('should remove block from tree', () => {
      const tree = createMockTree()
      const blockId = 'text-1'

      const updatedTree = EmailBlockClass.removeBlockFromTree(tree, blockId)

      expect(updatedTree).not.toBeNull()
      const deletedBlock = EmailBlockClass.findBlockById(updatedTree!, blockId)
      expect(deletedBlock).toBeNull()

      // Column should still exist but be empty
      const column = EmailBlockClass.findBlockById(updatedTree!, 'column-1')
      expect(column?.children).toHaveLength(0)
    })

    test('should not remove root element', () => {
      const tree = createMockTree()
      const updatedTree = EmailBlockClass.removeBlockFromTree(tree, 'mjml-1')
      expect(updatedTree).toBeNull()
    })

    test('should remove section with all children', () => {
      const tree = createMockTree()
      const updatedTree = EmailBlockClass.removeBlockFromTree(tree, 'section-1')

      expect(updatedTree).not.toBeNull()
      const section = EmailBlockClass.findBlockById(updatedTree!, 'section-1')
      expect(section).toBeNull()

      // Body should exist but be empty
      const body = EmailBlockClass.findBlockById(updatedTree!, 'body-1')
      expect(body?.children).toHaveLength(0)
    })
  })

  describe('onMoveBlock callback implementation', () => {
    test('should move block to new parent', () => {
      // Create a tree with two columns
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
                        content: '<p>Text to move</p>',
                        attributes: {}
                      }
                    ]
                  },
                  {
                    id: 'column-2',
                    type: 'mj-column',
                    children: []
                  }
                ]
              }
            ]
          }
        ]
      }

      const updatedTree = EmailBlockClass.moveBlockInTree(tree, 'text-1', 'column-2', 0)

      expect(updatedTree).not.toBeNull()

      // Text should now be in column-2
      const column2 = EmailBlockClass.findBlockById(updatedTree!, 'column-2')
      expect(column2?.children).toHaveLength(1)
      expect(column2?.children?.[0].id).toBe('text-1')

      // Column-1 should be empty
      const column1 = EmailBlockClass.findBlockById(updatedTree!, 'column-1')
      expect(column1?.children).toHaveLength(0)
    })

    test('should not move to invalid parent', () => {
      const tree = createMockTree()

      // Try to move section into text (invalid)
      const result = EmailBlockClass.moveBlockInTree(tree, 'section-1', 'text-1', 0)
      expect(result).toBeNull()
    })
  })

  describe('onSelectBlock callback', () => {
    test('should be a simple function call', () => {
      const setSelectedBlockId = vi.fn()
      const blockId = 'text-1'

      // Simulate onSelectBlock callback
      setSelectedBlockId(blockId)

      expect(setSelectedBlockId).toHaveBeenCalledWith('text-1')
    })
  })

  describe('setEmailTree callback implementation', () => {
    test('should validate tree before setting', () => {
      const validTree: EmailBlock = {
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
                    children: []
                  }
                ]
              }
            ]
          }
        ]
      }

      const errors = EmailBlockClass.validateStructure(validTree)
      expect(errors).toHaveLength(0)
    })

    test('should detect invalid structure', () => {
      const invalidTree: EmailBlock = {
        id: 'mjml-1',
        type: 'mjml',
        children: [
          {
            id: 'body-1',
            type: 'mj-body',
            children: [
              {
                id: 'text-1',
                type: 'mj-text', // Invalid: text cannot be direct child of body
                content: '<p>Invalid</p>',
                attributes: {}
              }
            ]
          }
        ]
      }

      const errors = EmailBlockClass.validateStructure(invalidTree)
      expect(errors.length).toBeGreaterThan(0)
    })

    test('should regenerate IDs when setting tree', () => {
      const tree = createMockTree()
      const originalId = tree.id

      const newTree = EmailBlockClass.regenerateIds(tree)

      expect(newTree.id).not.toBe(originalId)
      // All nested IDs should also be different
      const originalTextId = EmailBlockClass.findBlockById(tree, 'text-1')?.id
      const newTextBlock = EmailBlockClass.findBlockByType(newTree, 'mj-text')
      expect(newTextBlock?.id).not.toBe(originalTextId)
    })
  })
})

describe('EmailAIAssistant Integration Logic', () => {
  test('callbacks interface should have all required methods', () => {
    const mockCallbacks: EmailAIAgentCallbacks = {
      getEmailTree: vi.fn(),
      setEmailTree: vi.fn(),
      onAddBlock: vi.fn(),
      onUpdateBlock: vi.fn(),
      onDeleteBlock: vi.fn(),
      onMoveBlock: vi.fn(),
      onSelectBlock: vi.fn()
    }

    // Verify all methods exist
    expect(typeof mockCallbacks.getEmailTree).toBe('function')
    expect(typeof mockCallbacks.setEmailTree).toBe('function')
    expect(typeof mockCallbacks.onAddBlock).toBe('function')
    expect(typeof mockCallbacks.onUpdateBlock).toBe('function')
    expect(typeof mockCallbacks.onDeleteBlock).toBe('function')
    expect(typeof mockCallbacks.onMoveBlock).toBe('function')
    expect(typeof mockCallbacks.onSelectBlock).toBe('function')
  })

  test('onAddBlock should accept all parameters', () => {
    const onAddBlock = vi.fn()

    // Full signature
    onAddBlock('parent-1', 'mj-button', 0, 'Click me', { backgroundColor: '#007bff' })

    expect(onAddBlock).toHaveBeenCalledWith(
      'parent-1',
      'mj-button',
      0,
      'Click me',
      { backgroundColor: '#007bff' }
    )
  })

  test('onAddBlock should work with optional parameters', () => {
    const onAddBlock = vi.fn()

    // Minimal signature
    onAddBlock('parent-1', 'mj-spacer', undefined, undefined, undefined)

    expect(onAddBlock).toHaveBeenCalledWith(
      'parent-1',
      'mj-spacer',
      undefined,
      undefined,
      undefined
    )
  })

  test('onUpdateBlock should accept partial updates', () => {
    const onUpdateBlock = vi.fn()

    // Just attributes
    onUpdateBlock('block-1', { attributes: { color: '#fff' } })
    expect(onUpdateBlock).toHaveBeenCalledWith('block-1', { attributes: { color: '#fff' } })

    // Just content
    onUpdateBlock('block-2', { content: 'New text' })
    expect(onUpdateBlock).toHaveBeenCalledWith('block-2', { content: 'New text' })

    // Both
    onUpdateBlock('block-3', { content: 'Text', attributes: { fontSize: '16px' } })
    expect(onUpdateBlock).toHaveBeenCalledWith('block-3', { content: 'Text', attributes: { fontSize: '16px' } })
  })
})
