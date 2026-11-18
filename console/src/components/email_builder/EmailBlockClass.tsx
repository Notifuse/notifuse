import React from 'react'
import { v4 as uuidv4 } from 'uuid'
import type { DisplayCondition, EmailBlock, MJMLComponentType, SaveBlockHandler, SavedBlock, MergedBlockAttributes, BaseBlock } from './types'
import { MJML_COMPONENT_DEFAULTS, SOCIAL_INHERITED_ATTRIBUTES, mergeWithDefaults } from './mjml-defaults'
import { EmailBlockFactory } from './blocks/EmailBlockFactory'
import type { OnUpdateAttributesFunction, PreviewProps } from './blocks/BaseEmailBlock'

// CSS styles for hover effects
const hoverStyles = `
  .email-block-hover {
    position: relative;
  }
  
  .email-block-hover::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    pointer-events: none;
    opacity: 0;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.1), 0 2px 8px rgba(0, 0, 0, 0.15);
    transition: opacity 0.2s ease;
    z-index: 1;
  }
  
  /* Only show hover on the deepest element */
  .email-block-hover:hover::before {
    opacity: 1;
  }
  
  /* Hide hover effect on parents when any child is hovered */
  .email-block-hover:hover .email-block-hover:hover::before {
    opacity: 0;
  }
  
  /* Show hover only on the deepest hovered element */
  .email-block-hover:hover:not(:has(.email-block-hover:hover))::before {
    opacity: 1;
  }
  
  /* Fallback for browsers that don't support :has() */
  @supports not selector(:has(*)) {
    .email-block-hover:hover .email-block-hover::before {
      opacity: 0;
    }
    .email-block-hover:hover::before {
      opacity: 1;
    }
  }
  
  .email-block-hover.selected::before {
    box-shadow: 0px 0px 3px 1px #4E6CFF;
    opacity: 1;
  }
  
  .email-block-hover.selected:hover::before {
    box-shadow: 0px 0px 3px 1px #4E6CFF;
    opacity: 1;
  }
  
  /* Selection always visible, even when child is hovered */
  .email-block-hover.selected::before {
    opacity: 1 !important;
  }
`

// Inject styles into the document
if (typeof document !== 'undefined' && !document.getElementById('email-block-hover-styles')) {
  const styleElement = document.createElement('style')
  styleElement.id = 'email-block-hover-styles'
  styleElement.textContent = hoverStyles
  document.head.appendChild(styleElement)
}

/**
 * EmailBlock utility class that provides methods for working with MJML components
 */
export class EmailBlockClass {
  private block: EmailBlock

  constructor(block: EmailBlock) {
    this.block = block
  }

  /**
   * Get the FontAwesome icon for the component type
   */
  getIcon(parentType?: MJMLComponentType): React.ReactNode {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(this.block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(this.block)
        return blockInstance.getIcon(parentType)
      }
    } catch (error) {
      // Fall back to legacy implementation
      console.error(error)
    }

    return null
  }

  /**
   * Get human-readable label for the component
   */
  getLabel(index?: number): string {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(this.block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(this.block)
        return blockInstance.getLabel(index)
      }
    } catch (error) {
      console.error(error)
    }
    return this.block.type
  }

  /**
   * Get header block description with illustration for popover display
   */
  getHeaderBlockDescription(): React.ReactNode | null {
    switch (this.block.type) {
      case 'mj-breakpoint':
        return (
          <div>
            <div style={{ marginBottom: 12 }}>
              Set responsive breakpoint width for mobile optimization and layout control
            </div>
            <div
              style={{
                width: 60,
                height: 30,
                border: '2px solid #52c41a',
                borderRadius: 4,
                backgroundColor: '#f6ffed',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                position: 'relative',
                margin: '0 auto'
              }}
            >
              <div
                style={{
                  width: 40,
                  height: 20,
                  border: '1px solid #52c41a',
                  borderRadius: 2,
                  backgroundColor: '#f6ffed',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center'
                }}
              >
                <div style={{ fontSize: 8, color: '#52c41a', fontWeight: 'bold' }}>📱</div>
              </div>
              <div
                style={{
                  position: 'absolute',
                  right: 2,
                  top: 2,
                  width: 4,
                  height: 4,
                  backgroundColor: '#52c41a',
                  borderRadius: '50%'
                }}
              />
            </div>
          </div>
        )

      case 'mj-font':
        return (
          <div>
            <div style={{ marginBottom: 12 }}>
              Import custom fonts from external sources like Google Fonts for typography
            </div>
            <div
              style={{
                width: 60,
                height: 30,
                border: '2px solid #1890ff',
                borderRadius: 4,
                backgroundColor: '#e6f7ff',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexDirection: 'column',
                position: 'relative',
                margin: '0 auto'
              }}
            >
              <div style={{ fontSize: 10, color: '#1890ff', fontWeight: 'bold', lineHeight: 1 }}>
                Aa
              </div>
              <div style={{ fontSize: 6, color: '#69c0ff', marginTop: 1 }}>Font</div>
            </div>
          </div>
        )

      case 'mj-style':
        return (
          <div>
            <div style={{ marginBottom: 12 }}>
              Add custom CSS styles to enhance email appearance and layout
            </div>
            <div
              style={{
                width: 60,
                height: 30,
                border: '2px solid #eb2f96',
                borderRadius: 4,
                backgroundColor: '#fff0f6',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                position: 'relative',
                margin: '0 auto'
              }}
            >
              <div
                style={{
                  width: 40,
                  height: 16,
                  backgroundColor: '#ffadd6',
                  borderRadius: 2,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 8,
                  color: '#eb2f96',
                  fontWeight: 'bold'
                }}
              >
                CSS
              </div>
              <div
                style={{
                  position: 'absolute',
                  right: 3,
                  top: 3,
                  fontSize: 8,
                  color: '#eb2f96'
                }}
              >
                ✨
              </div>
            </div>
          </div>
        )

      default:
        return null
    }
  }

  /**
   * Render the settings panel using the new block architecture
   */
  getRenderSettingsPanel(
    _currentAttributes: Record<string, unknown>,
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, unknown>
  ): React.ReactNode {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(this.block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(this.block)
        return blockInstance.renderSettingsPanel(onUpdate, blockDefaults as MergedBlockAttributes)
      }
    } catch (error) {
      console.error(`Error rendering settings panel for ${this.block.type}:`, error)
    }

    return null
  }

  /**
   * Get default attributes for the component type
   */
  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS[this.block.type] || {}
  }

  /**
   * Get rendered preview of the component (legacy method for unmigrated blocks)
   */
  getEdit(props: PreviewProps): React.ReactNode {
    return EmailBlockClass.renderEmailBlock(
      this.block,
      props.attributeDefaults || {},
      props.selectedBlockId || null,
      props.onSelectBlock,
      props.emailTree,
      props.onUpdateBlock,
      props.onCloneBlock,
      props.onDeleteBlock,
      props.onSaveBlock
    )
  }

  /**
   * Static method to create an EmailBlockClass instance from a block
   */
  static from(block: EmailBlock): EmailBlockClass {
    return new EmailBlockClass(block)
  }

  /**
   * Check if this component can contain children
   */
  canHaveChildren(): boolean {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(this.block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(this.block)
        return blockInstance.canHaveChildren()
      }
    } catch {
      // Fall back to legacy implementation
    }

    // Legacy implementation for unmigrated blocks only
    const childlessComponents: MJMLComponentType[] = ['mj-preview', 'mj-title']
    return !childlessComponents.includes(this.block.type)
  }

  /**
   * Get valid child component types for this component
   */
  getValidChildTypes(): MJMLComponentType[] {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(this.block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(this.block)
        return blockInstance.getValidChildTypes()
      }
    } catch {
      // Fall back to legacy implementation
    }

    // Legacy implementation for unmigrated blocks only
    const validChildren: Partial<Record<MJMLComponentType, MJMLComponentType[]>> = {
      'mj-head': ['mj-attributes', 'mj-preview', 'mj-title'],
      'mj-attributes': [],
      'mj-preview': [],
      'mj-title': []
    }

    return validChildren[this.block.type] || []
  }

  /**
   * Check if a component type can be dropped into this component
   */
  canAcceptChild(childType: MJMLComponentType): boolean {
    return this.getValidChildTypes().includes(childType)
  }

  /**
   * Static utility method to find a block by ID in a tree
   */
  static findBlockById(tree: EmailBlock, id: string): EmailBlock | null {
    if (tree.id === id) return tree

    if (tree.children) {
      for (const child of tree.children) {
        const found = EmailBlockClass.findBlockById(child, id)
        if (found) return found
      }
    }

    return null
  }

  /**
   * Static utility method to find the first block of a specific type in a tree
   */
  static findBlockByType(tree: EmailBlock, type: MJMLComponentType): EmailBlock | null {
    if (tree.type === type) return tree

    if (tree.children) {
      for (const child of tree.children) {
        const found = EmailBlockClass.findBlockByType(child, type)
        if (found) return found
      }
    }

    return null
  }

  /**
   * Static utility method to find all blocks of a specific type in a tree
   */
  static findAllBlocksByType(tree: EmailBlock, type: MJMLComponentType): EmailBlock[] {
    const blocks: EmailBlock[] = []

    const traverse = (node: EmailBlock) => {
      if (node.type === type) {
        blocks.push(node)
      }

      if (node.children) {
        for (const child of node.children) {
          traverse(child)
        }
      }
    }

    traverse(tree)
    return blocks
  }

  /**
   * Static utility method to generate a unique UUID for block IDs
   */
  static generateId(): string {
    return uuidv4()
  }

  /**
   * Static utility method to generate a new block with defaults from both global and mj-attributes
   */
  static createBlock(
    type: MJMLComponentType,
    id?: string,
    content?: string,
    emailTree?: EmailBlock
  ): EmailBlock {
    const globalDefaults = { ...(MJML_COMPONENT_DEFAULTS[type] || {}) }
    // A new social element takes what its mj-social passes rather than writing the same values as its own,
    // which would win over what the mj-social sets (see SOCIAL_INHERITED_ATTRIBUTES)
    if (type === 'mj-social-element') {
      for (const key of [...SOCIAL_INHERITED_ATTRIBUTES, 'padding']) {
        delete (globalDefaults as Record<string, unknown>)[key]
      }
    }

    // Extract mj-attributes defaults if emailTree is provided
    let attributeDefaults: Record<string, unknown> = {}
    if (emailTree) {
      const mjAttributesDefaults = EmailBlockClass.extractAttributeDefaults(emailTree)
      attributeDefaults = mjAttributesDefaults[type] || {}
    }

    // Merge defaults: globalDefaults < attributeDefaults
    const mergedAttributes = { ...globalDefaults, ...attributeDefaults }

    // Create base block structure with mutable type for building
    const block: {
      id: string
      type: MJMLComponentType
      attributes: Record<string, unknown>
      children?: BaseBlock[]
      content?: string
    } = {
      id: id || EmailBlockClass.generateId(),
      type,
      attributes: mergedAttributes
    }

    // Add children for container blocks
    if (
      type !== 'mj-text' &&
      type !== 'mj-button' &&
      type !== 'mj-image' &&
      type !== 'mj-preview' &&
      type !== 'mj-title' &&
      type !== 'mj-style' &&
      type !== 'mj-raw' &&
      type !== 'mj-liquid'
    ) {
      block.children = []
    }

    // Special handling for blocks that support content
    if (
      type === 'mj-text' ||
      type === 'mj-button' ||
      type === 'mj-title' ||
      type === 'mj-preview' ||
      type === 'mj-raw' ||
      type === 'mj-liquid' ||
      type === 'mj-style'
    ) {
      // For mj-text blocks, ensure content is wrapped in <p> tags (Tiptap always wraps in <p>)
      if (type === 'mj-text') {
        if (content) {
          // Check if content is already wrapped in HTML tags
          const isWrappedInHtml = /^\s*</.test(content)
          block.content = isWrappedInHtml ? content : `<p>${content}</p>`
        } else {
          // Default content for new text blocks
          block.content = '<p></p>'
        }
      } else {
        // For other content-supporting blocks, use content as-is or initialize to empty string
        // This ensures the content field exists on the block for mj-raw and mj-style blocks
        block.content = content ?? ''
      }
    }

    // Add default social elements for mj-social blocks
    if (type === 'mj-social') {
      const defaultSocialElements: BaseBlock[] = [
        {
          id: EmailBlockClass.generateId(),
          type: 'mj-social-element' as MJMLComponentType,
          attributes: {
            name: 'facebook-noshare',
            href: 'https://facebook.com',
            backgroundColor: '#3b5998'
          },
          children: []
        },
        {
          id: EmailBlockClass.generateId(),
          type: 'mj-social-element' as MJMLComponentType,
          attributes: {
            name: 'instagram',
            href: 'https://instagram.com',
            backgroundColor: '#E4405F'
          },
          children: []
        },
        {
          id: EmailBlockClass.generateId(),
          type: 'mj-social-element' as MJMLComponentType,
          attributes: {
            name: 'x-noshare',
            href: 'https://x.com',
            backgroundColor: '#000000'
          },
          children: []
        }
      ]

      block.children = defaultSocialElements
    }

    return block as EmailBlock
  }

  /**
   * Static utility method to validate MJML structure
   */
  static validateStructure(tree: EmailBlock): string[] {
    const errors: string[] = []

    const validate = (block: EmailBlock, path: string = '') => {
      const blockClass = EmailBlockClass.from(block)
      const currentPath = path ? `${path} > ${blockClass.getLabel()}` : blockClass.getLabel()

      // Check if children are valid
      if (block.children) {
        for (const child of block.children) {
          if (!blockClass.canAcceptChild(child.type)) {
            errors.push(
              `Invalid child: ${child.type} cannot be placed inside ${block.type} at ${currentPath}`
            )
          }
          validate(child, currentPath)
        }
      }

      // Check required attributes for specific blocks
      if (block.type === 'mj-image' && block.attributes && !('src' in block.attributes)) {
        errors.push(`Missing required 'src' attribute for image at ${currentPath}`)
      }
    }

    validate(tree)
    return errors
  }

  /**
   * Static utility method to remove a block from the tree
   */
  static removeBlockFromTree(tree: EmailBlock, blockId: string): EmailBlock | null {
    // Create deep copy to avoid mutation
    const newTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    const removeBlock = (parent: EmailBlock, targetId: string): boolean => {
      if (parent.children) {
        const index = parent.children.findIndex((child) => child.id === targetId)
        if (index !== -1) {
          parent.children.splice(index, 1)
          return true
        }

        // Recursively search in children
        for (const child of parent.children) {
          if (removeBlock(child, targetId)) {
            return true
          }
        }
      }
      return false
    }

    // Don't allow removing the root element
    if (newTree.id === blockId) {
      return null
    }

    const found = removeBlock(newTree, blockId)
    return found ? newTree : null
  }

  /**
   * Static utility method to insert a block into the tree at a specific position
   */
  static insertBlockIntoTree(
    tree: EmailBlock,
    parentId: string,
    block: EmailBlock,
    position: number
  ): EmailBlock | null {
    // Create deep copy to avoid mutation
    const newTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    const insertBlock = (current: EmailBlock, targetParentId: string): boolean => {
      if (current.id === targetParentId) {
        // Use a mutable children array for manipulation
        const children = (current.children || []) as BaseBlock[]

        // Ensure position is within bounds
        const insertPosition = Math.max(0, Math.min(position, children.length))
        // Use type assertion since EmailBlock children arrays are readonly by type
        children.splice(insertPosition, 0, block as BaseBlock)

        // Assign back to current
        ;(current as { children: BaseBlock[] }).children = children
        return true
      }

      if (current.children) {
        for (const child of current.children) {
          if (insertBlock(child, targetParentId)) {
            return true
          }
        }
      }
      return false
    }

    const found = insertBlock(newTree, parentId)
    return found ? newTree : null
  }

  /**
   * Static utility method to move a block within the tree
   */
  static moveBlockInTree(
    tree: EmailBlock,
    blockId: string,
    newParentId: string,
    position: number
  ): EmailBlock | null {
    // First, find and extract the block to move
    const blockToMove = EmailBlockClass.findBlockById(tree, blockId)
    if (!blockToMove) {
      return null
    }

    // Validate that the move is allowed
    const newParent = EmailBlockClass.findBlockById(tree, newParentId)
    if (!newParent) {
      return null
    }

    const parentClass = EmailBlockClass.from(newParent)
    if (!parentClass.canAcceptChild(blockToMove.type)) {
      console.warn(`Cannot move ${blockToMove.type} into ${newParent.type}`)
      return null
    }

    // Create a deep copy of the block to move
    const blockCopy = JSON.parse(JSON.stringify(blockToMove)) as EmailBlock

    // Remove the block from its current position
    const treeWithoutBlock = EmailBlockClass.removeBlockFromTree(tree, blockId)
    if (!treeWithoutBlock) {
      return null
    }

    // Insert the block at the new position
    const finalTree = EmailBlockClass.insertBlockIntoTree(
      treeWithoutBlock,
      newParentId,
      blockCopy,
      position
    )

    return finalTree
  }

  /**
   * Extract default attributes from mj-attributes blocks
   */
  static extractAttributeDefaults(
    mjmlTree: EmailBlock
  ): Partial<Record<MJMLComponentType, Record<string, unknown>>> {
    const defaults: Partial<Record<MJMLComponentType, Record<string, unknown>>> = {}

    // Find mj-head block
    const mjHead = mjmlTree.children?.find((child) => child.type === 'mj-head')
    if (!mjHead) return defaults

    // Find mj-attributes block within mj-head
    const mjAttributes = mjHead.children?.find((child) => child.type === 'mj-attributes')
    if (!mjAttributes || !mjAttributes.children) return defaults

    // Extract defaults for each component type
    mjAttributes.children.forEach((attributeBlock) => {
      if (attributeBlock.attributes) {
        defaults[attributeBlock.type as MJMLComponentType] = attributeBlock.attributes as Record<string, unknown>
      }
    })

    return defaults
  }

  /**
   * Merge component attributes with both global defaults and mj-attributes defaults
   */
  static mergeWithAllDefaults(
    componentType: MJMLComponentType,
    blockAttributes: Record<string, unknown> | undefined = {},
    attributeDefaults: Partial<Record<MJMLComponentType, Record<string, unknown>>> = {}
  ): MergedBlockAttributes {
    const globalDefaults = mergeWithDefaults(componentType, {})
    const customDefaults = attributeDefaults[componentType] || {}

    // Priority: blockAttributes > customDefaults > globalDefaults
    return { ...globalDefaults, ...customDefaults, ...blockAttributes } as MergedBlockAttributes
  }

  /**
   * Static method to render an email block and all its children recursively
   * using the new block class architecture
   */
  static renderEmailBlock(
    block: EmailBlock,
    attributeDefaults: Partial<Record<MJMLComponentType, Record<string, unknown>>>,
    selectedBlockId: string | null,
    onSelectBlock: (blockId: string) => void,
    emailTree: EmailBlock,
    onUpdateBlock: (blockId: string, updates: EmailBlock) => void,
    onCloneBlock: (blockId: string) => void,
    onDeleteBlock: (blockId: string) => void,
    onSaveBlock: SaveBlockHandler,
    savedBlocks?: SavedBlock[]
  ): React.ReactNode {
    // Try to use the new block class architecture first
    try {
      if (EmailBlockFactory.hasBlockType(block.type)) {
        const blockInstance = EmailBlockFactory.createBlock(block)
        return blockInstance.getEdit({
          selectedBlockId,
          onSelectBlock,
          onUpdateBlock,
          onCloneBlock,
          onDeleteBlock,
          attributeDefaults,
          emailTree,
          onSaveBlock,
          savedBlocks
        })
      }
    } catch (error) {
      console.warn(`Error using new block class for ${block.type}, falling back to legacy:`, error)
    }

    // Legacy implementation for unmigrated blocks only
    switch (block.type) {
      // MJML Head components (typically not rendered in preview)
      case 'mj-head':
      case 'mj-attributes':
      case 'mj-preview':
      case 'mj-title':
        return null

      default:
        // For migrated blocks, this shouldn't be reached since the new architecture
        // handles them first. For unmigrated blocks, return null as fallback.
        console.warn(`No rendering implementation for block type: ${block.type}`)
        return null
    }
  }

  /**
   * Static utility method to regenerate all IDs in an email tree with UUIDs
   */
  static regenerateIds(tree: EmailBlock): EmailBlock {
    const newTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    const regenerateIdsRecursive = (block: EmailBlock): void => {
      block.id = EmailBlockClass.generateId()
      if (block.children) {
        block.children.forEach(regenerateIdsRecursive)
      }
    }

    regenerateIdsRecursive(newTree)
    return newTree
  }

  /**
   * Applies a partial update (the AI assistant's updateBlock) to a copy of tree: merges
   * attributes, replaces content, and sets or removes (null) the display condition,
   * leaving every other field of the block as it was. Returns tree itself when the
   * block is not found.
   */
  static mergeBlockUpdate(
    tree: EmailBlock,
    blockId: string,
    updates: Partial<Omit<EmailBlock, 'display_condition'>> & {
      display_condition?: DisplayCondition | null
    }
  ): EmailBlock {
    const updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock
    const block = EmailBlockClass.findBlockById(updatedTree, blockId)
    if (!block) return tree
    if (updates.attributes) {
      block.attributes = { ...block.attributes, ...updates.attributes }
    }
    if (updates.content !== undefined) {
      block.content = updates.content
    }
    if (updates.display_condition === null) {
      delete block.display_condition
    } else if (updates.display_condition !== undefined) {
      block.display_condition = updates.display_condition
    }
    return updatedTree
  }

  /**
   * Static utility method to check if a block is a descendant of a specific parent type
   */
  static isChildOf(tree: EmailBlock, blockId: string, ancestorType: MJMLComponentType): boolean {
    const findBlockPath = (
      current: EmailBlock,
      targetId: string,
      path: EmailBlock[] = []
    ): EmailBlock[] | null => {
      const currentPath = [...path, current]

      if (current.id === targetId) {
        return currentPath
      }

      if (current.children) {
        for (const child of current.children) {
          const result = findBlockPath(child, targetId, currentPath)
          if (result) return result
        }
      }

      return null
    }

    const blockPath = findBlockPath(tree, blockId)
    if (!blockPath) return false

    // Check if any ancestor in the path has the specified type
    return blockPath.some((ancestor) => ancestor.type === ancestorType)
  }

  /**
   * Static utility method to get all ancestor block IDs for a given block
   */
  static getAncestorIds(tree: EmailBlock, targetId: string): string[] {
    const findBlockPath = (
      current: EmailBlock,
      targetId: string,
      path: string[] = []
    ): string[] | null => {
      if (current.id === targetId) {
        return path
      }

      if (current.children) {
        for (const child of current.children) {
          const result = findBlockPath(child, targetId, [...path, current.id])
          if (result) return result
        }
      }

      return null
    }

    return findBlockPath(tree, targetId) || []
  }

  /**
   * Static utility method to clean up font family references when a font import is removed
   * This traverses the tree and resets any fontFamily attributes that match the removed font to a default
   */
  static cleanupFontReferences(
    tree: EmailBlock,
    removedFontName: string,
    defaultFont: string = 'Arial, sans-serif'
  ): EmailBlock {
    // Create a deep copy to avoid mutation
    const cleanedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    const traverse = (node: EmailBlock): void => {
      // Check if this node has fontFamily attribute that matches the removed font
      if (node.attributes && 'fontFamily' in node.attributes) {
        const currentFontFamily = node.attributes.fontFamily
        if (currentFontFamily === removedFontName) {
          console.log(
            `Resetting fontFamily from "${removedFontName}" to "${defaultFont}" for block ${node.id} (${node.type})`
          )
          node.attributes.fontFamily = defaultFont
        }
      }

      // Recursively traverse children
      if (node.children) {
        for (const child of node.children) {
          traverse(child)
        }
      }
    }

    traverse(cleanedTree)
    return cleanedTree
  }
}
