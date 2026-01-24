import React from 'react'
import { v4 as uuidv4 } from 'uuid'
import type { EmailBlock, MJMLComponentType, SaveOperation, SavedBlock, MergedBlockAttributes, BaseBlock } from './types'
import { MJML_COMPONENT_DEFAULTS, mergeWithDefaults } from './mjml-defaults'
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
    const globalDefaults = MJML_COMPONENT_DEFAULTS[type] || {}

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
      type !== 'mj-raw'
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
            name: 'facebook',
            href: 'https://facebook.com',
            backgroundColor: '#3b5998',
            borderRadius: '3px'
          },
          children: []
        },
        {
          id: EmailBlockClass.generateId(),
          type: 'mj-social-element' as MJMLComponentType,
          attributes: {
            name: 'instagram',
            href: 'https://instagram.com',
            backgroundColor: '#E4405F',
            borderRadius: '3px'
          },
          children: []
        },
        {
          id: EmailBlockClass.generateId(),
          type: 'mj-social-element' as MJMLComponentType,
          attributes: {
            name: 'x',
            href: 'https://x.com',
            backgroundColor: '#000000',
            borderRadius: '3px'
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
    onSaveBlock: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void,
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
   * Create the initial email template using EmailBlockClass methods
   * This ensures proper defaults and column width calculations
   */
  static GetInitialTemplate(): EmailBlock {
    // Import defaults directly from the already imported constants
    const defaults = MJML_COMPONENT_DEFAULTS

    // Create root MJML block
    const mjml = EmailBlockClass.createBlock('mjml', 'mjml-1')

    // Create head block
    const head = EmailBlockClass.createBlock('mj-head', 'head-1')

    // Create mj-attributes block with defaults
    const attributes = EmailBlockClass.createBlock('mj-attributes', 'attributes-1')

    // Create attribute defaults for each component type
    const textDefaults = EmailBlockClass.createBlock('mj-text', 'text-defaults-1')
    textDefaults.attributes = {
      ...defaults['mj-text'],
      fontSize: '16px',
      color: '#333333',
      lineHeight: '1.6',
      fontFamily: 'Arial, sans-serif',
      align: 'left',
      paddingTop: '10px',
      paddingRight: '25px',
      paddingBottom: '10px',
      paddingLeft: '25px'
    }

    const buttonDefaults = EmailBlockClass.createBlock('mj-button', 'button-defaults-1')
    buttonDefaults.attributes = {
      ...defaults['mj-button'],
      backgroundColor: '#007bff',
      color: '#ffffff',
      borderRadius: '4px',
      fontSize: '16px',
      fontWeight: 'bold',
      innerPadding: '12px 24px',
      lineHeight: '120%',
      paddingTop: '15px',
      paddingRight: '25px',
      paddingBottom: '15px',
      paddingLeft: '25px',
      target: '_blank',
      verticalAlign: 'middle',
      align: 'center',
      textDecoration: 'none',
      textTransform: 'none'
    }

    const imageDefaults = EmailBlockClass.createBlock('mj-image', 'image-defaults-1')
    imageDefaults.attributes = {
      ...defaults['mj-image'],
      align: 'center',
      src: 'https://placehold.co/150x60/E3F2FD/1976D2?font=playfair-display&text=LOGO'
    }

    const sectionDefaults = EmailBlockClass.createBlock('mj-section', 'section-defaults-1')
    sectionDefaults.attributes = {
      ...defaults['mj-section'],
      paddingTop: '20px',
      paddingBottom: '20px',
      textAlign: 'center',
      fullWidth: 'full-width'
    }

    const columnDefaults = EmailBlockClass.createBlock('mj-column', 'column-defaults-1')
    columnDefaults.attributes = {
      ...defaults['mj-column'],
      width: '100%'
    }

    // Add defaults to attributes block
    ;(attributes as BaseBlock).children = [
      textDefaults,
      buttonDefaults,
      imageDefaults,
      sectionDefaults,
      columnDefaults
    ]

    // Create preview block
    const preview = EmailBlockClass.createBlock(
      'mj-preview',
      'preview-1',
      'Welcome! See how sections provide borders and groups prevent mobile stacking.'
    )

    // Add attributes first, then preview to head
    ;(head as BaseBlock).children = [attributes, preview]

    // Create body block
    const body = EmailBlockClass.createBlock('mj-body', 'body-1')
    body.attributes = { width: '600px', backgroundColor: '#f8f9fa' }

    // Build the email tree so far
    ;(mjml as BaseBlock).children = [head, body]
    let emailTree = mjml

    // Create wrapper with 20px padding around the sections
    const wrapper = EmailBlockClass.createBlock('mj-wrapper', 'wrapper-1')
    wrapper.attributes = {
      paddingTop: '20px',
      paddingRight: '20px',
      paddingBottom: '20px',
      paddingLeft: '20px'
    }

    // Insert wrapper into body
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'body-1', wrapper, 0)!

    // Create full-width hero section with background image (inside wrapper)
    const heroSection = EmailBlockClass.createBlock('mj-section', 'hero-section-1')
    heroSection.attributes = {
      fullWidth: 'full-width',
      backgroundUrl:
        'https://images.unsplash.com/photo-1495419597644-19934b6b7c40?w=500&auto=format&fit=crop&q=60&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxzZWFyY2h8N3x8YmVhY2glMjBwYXN0ZWx8ZW58MHx8MHx8fDA%3D',
      backgroundSize: 'cover',
      backgroundRepeat: 'no-repeat',
      backgroundPosition: 'center center',
      paddingTop: '80px',
      paddingRight: '20px',
      paddingBottom: '80px',
      paddingLeft: '20px',
      textAlign: 'center'
    }

    // Insert hero section into wrapper first
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'wrapper-1', heroSection, 0)!

    // Create hero column
    const heroColumn = EmailBlockClass.createBlock('mj-column', 'hero-column-1')
    heroColumn.attributes = { width: '100%' }

    // Insert hero column into hero section
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'hero-section-1', heroColumn, 0)!

    // Add logo image to hero
    const logo = EmailBlockClass.createBlock('mj-image', 'logo-1', undefined, emailTree)
    logo.attributes = {
      ...logo.attributes,
      src: 'https://placehold.co/150x60/E3F2FD/1976D2?font=playfair-display&text=LOGO',
      width: '150px',
      paddingBottom: '20px'
    }

    // Add hero title with styling moved to attributes
    const heroTitle = EmailBlockClass.createBlock('mj-text', 'hero-title-1', undefined, emailTree)
    heroTitle.content = 'Beautiful Email Templates'
    heroTitle.attributes = {
      ...heroTitle.attributes,
      color: '#ffffff',
      align: 'center',
      paddingBottom: '20px',
      paddingTop: '0px',
      paddingLeft: '20px',
      paddingRight: '20px',
      fontSize: '32px',
      fontWeight: 'bold',
      lineHeight: '1.2'
    }

    // Add hero subtitle with styling moved to attributes
    const heroSubtitle = EmailBlockClass.createBlock(
      'mj-text',
      'hero-subtitle-1',
      undefined,
      emailTree
    )
    heroSubtitle.content =
      'Create stunning, responsive emails with our powerful MJML builder'
    heroSubtitle.attributes = {
      ...heroSubtitle.attributes,
      color: '#000000',
      align: 'center',
      paddingBottom: '30px',
      paddingTop: '0px',
      paddingLeft: '40px',
      paddingRight: '40px',
      fontSize: '20px',
      fontWeight: '500',
      lineHeight: '28px'
    }

    // Add hero call-to-action button
    const heroButton = EmailBlockClass.createBlock(
      'mj-button',
      'hero-button-1',
      'Get Started Today',
      emailTree
    )
    heroButton.attributes = {
      ...heroButton.attributes,
      href: '#',
      backgroundColor: '#EC407A',
      color: '#ffffff',
      borderRadius: '30px',
      fontSize: '18px',
      fontWeight: 'bold',
      innerPadding: '16px 32px',
      lineHeight: '120%',
      paddingTop: '0px',
      paddingRight: '25px',
      paddingBottom: '0px',
      paddingLeft: '25px',
      target: '_blank',
      verticalAlign: 'middle',
      align: 'center',
      textDecoration: 'none',
      textTransform: 'none'
    }

    // Insert hero content into hero column (logo first, then title, subtitle, button)
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'hero-column-1', logo, 0)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'hero-column-1', heroTitle, 1)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'hero-column-1', heroSubtitle, 2)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'hero-column-1', heroButton, 3)!

    // Create title section
    const titleSection = EmailBlockClass.createBlock('mj-section', 'title-section')
    titleSection.attributes = {
      backgroundColor: '#ffffff',
      borderRadius: '8px',
      paddingTop: '25px',
      paddingRight: '25px',
      paddingBottom: '25px',
      paddingLeft: '25px'
    }

    // Insert title section into wrapper (at position 1, after hero)
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'wrapper-1', titleSection, 1)!

    // Create column for title section
    const titleColumn = EmailBlockClass.createBlock('mj-column', 'title-column')
    titleColumn.attributes = { width: '100%' }

    // Insert column into title section
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'title-section', titleColumn, 0)!

    // Add title text with styling moved to attributes
    const title = EmailBlockClass.createBlock('mj-text', 'title-1', undefined, emailTree)
    title.content = 'Welcome {{name}} to MJML email builder'
    title.attributes = {
      ...title.attributes,
      color: '#EC407A',
      align: 'center',
      paddingBottom: '15px',
      fontSize: '24px',
      fontWeight: 'bold',
      lineHeight: '1.3'
    }

    // Add explanation text with styling moved to attributes
    const explanation = EmailBlockClass.createBlock(
      'mj-text',
      'explanation-1',
      undefined,
      emailTree
    )
    explanation.content =
      "This section demonstrates MJML components. Below you'll see a group that prevents its columns from stacking on mobile devices."
    explanation.attributes = {
      ...explanation.attributes,
      align: 'center',
      color: '#666666',
      paddingBottom: '25px'
    }

    // Add button
    const button1 = EmailBlockClass.createBlock(
      'mj-button',
      'button-1',
      'Learn More About MJML',
      emailTree
    )
    button1.attributes = {
      ...button1.attributes,
      href: 'https://documentation.mjml.io/',
      backgroundColor: '#EC407A',
      paddingBottom: '30px'
    }

    // Insert content blocks into title column
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'title-column', title, 0)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'title-column', explanation, 1)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'title-column', button1, 2)!

    // Add divider block directly to title column with padding for better selectability
    const divider = EmailBlockClass.createBlock('mj-divider', 'divider-1', undefined, emailTree)
    divider.attributes = {
      ...divider.attributes,
      borderColor: '#dee2e6',
      borderStyle: 'solid',
      borderWidth: '2px',
      width: '50%',
      align: 'center',
      paddingTop: '30px',
      paddingRight: '25px',
      paddingBottom: '30px',
      paddingLeft: '25px'
    }

    // Insert divider into title column after the button
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'title-column', divider, 3)!

    // Create section with group content (removed green borders)
    const section1 = EmailBlockClass.createBlock('mj-section', 'section-1')
    section1.attributes = {
      backgroundColor: '#ffffff',
      borderRadius: '8px',
      paddingTop: '25px',
      paddingRight: '25px',
      paddingBottom: '25px',
      paddingLeft: '25px'
    }

    // Insert section1 into wrapper (now at position 2, after title section)
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'wrapper-1', section1, 2)!

    // Create group to prevent mobile stacking
    const group1 = EmailBlockClass.createBlock('mj-group', 'group-1')
    group1.attributes = {
      width: '100%',
      backgroundColor: '#f8f9fa'
    }
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'section-1', group1, 0)!

    // Add first column to group
    const column2 = EmailBlockClass.createBlock('mj-column', 'column-2')
    column2.attributes = { width: '50%' }
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'group-1', column2, 0)!

    // Add second column to group
    const column3 = EmailBlockClass.createBlock('mj-column', 'column-3')
    column3.attributes = { width: '50%' }
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'group-1', column3, 1)!

    // Add content to left column with styling moved to attributes
    const leftTitle = EmailBlockClass.createBlock('mj-text', 'left-title-1', undefined, emailTree)
    leftTitle.content = 'Left Column'
    leftTitle.attributes = {
      ...leftTitle.attributes,
      color: '#EC407A',
      align: 'center',
      paddingBottom: '10px',
      fontSize: '18px',
      fontWeight: 'bold',
      lineHeight: '1.4'
    }

    const leftText = EmailBlockClass.createBlock('mj-text', 'left-text-1', undefined, emailTree)
    leftText.content =
      'This column stays side-by-side with the right column even on mobile devices because both columns are inside an mj-group element.'
    leftText.attributes = {
      ...leftText.attributes,
      align: 'center',
      color: '#666666',
      fontSize: '14px'
    }

    // Add content to right column with styling moved to attributes
    const rightTitle = EmailBlockClass.createBlock('mj-text', 'right-title-1', undefined, emailTree)
    rightTitle.content = 'Right Column'
    rightTitle.attributes = {
      ...rightTitle.attributes,
      color: '#EC407A',
      align: 'center',
      paddingBottom: '10px',
      fontSize: '18px',
      fontWeight: 'bold',
      lineHeight: '1.4'
    }

    const rightText = EmailBlockClass.createBlock('mj-text', 'right-text-1', undefined, emailTree)
    rightText.content =
      'Groups prevent the default mobile stacking behavior. Without a group, these columns would stack vertically on mobile for better readability.'
    rightText.attributes = {
      ...rightText.attributes,
      align: 'center',
      color: '#666666',
      fontSize: '14px'
    }

    // Insert content into columns
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'column-2', leftTitle, 0)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'column-2', leftText, 1)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'column-3', rightTitle, 0)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'column-3', rightText, 1)!

    // Add a demo mj-raw block at the end
    const rawBlock = EmailBlockClass.createBlock('mj-raw', 'raw-demo-1', undefined, emailTree)
    rawBlock.content = `<!-- Custom CSS for email clients -->
<style type="text/css">
  .custom-highlight {
    background: linear-gradient(45deg, #ff6b6b, #4ecdc4);
    padding: 2px 6px;
    border-radius: 3px;
    color: white;
    font-weight: bold;
  }
</style>

<!-- Custom HTML structure -->
<div style="text-align: center; padding: 20px; background-color: #f0f8ff; margin: 20px 0; border-radius: 8px;">
  <p style="margin: 0; font-size: 14px; color: #333;">
    This is <span class="custom-highlight">custom HTML</span> content from an mj-raw block!
  </p>
  <p style="margin: 8px 0 0 0; font-size: 12px; color: #666;">
    Perfect for custom styling, special HTML structures, or advanced layouts.
  </p>
</div>`

    // Insert the raw block into the wrapper at the end (position 3, after title-section and section-1)
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'wrapper-1', rawBlock, 3)!

    // Create social media section at the bottom
    const socialSection = EmailBlockClass.createBlock('mj-section', 'social-section-1')
    socialSection.attributes = {
      backgroundColor: '#ffffff',
      borderRadius: '8px',
      paddingTop: '25px',
      paddingRight: '25px',
      paddingBottom: '25px',
      paddingLeft: '25px'
    }

    // Insert social section into wrapper (position 4, after raw block)
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'wrapper-1', socialSection, 4)!

    // Create column for social section
    const socialColumn = EmailBlockClass.createBlock('mj-column', 'social-column-1')
    socialColumn.attributes = { width: '100%' }

    // Insert column into social section
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'social-section-1', socialColumn, 0)!

    // Add social section title with styling moved to attributes
    const socialTitle = EmailBlockClass.createBlock(
      'mj-text',
      'social-title-1',
      undefined,
      emailTree
    )
    socialTitle.content = 'Connect With Us'
    socialTitle.attributes = {
      ...socialTitle.attributes,
      color: '#EC407A',
      align: 'center',
      paddingBottom: '20px',
      fontSize: '24px',
      fontWeight: 'bold',
      lineHeight: '1.3'
    }

    // Create mj-social block (will automatically get Facebook, Instagram, and X elements)
    const socialBlock = EmailBlockClass.createBlock('mj-social', 'social-1', undefined, emailTree)
    socialBlock.attributes = {
      ...socialBlock.attributes,
      mode: 'horizontal',
      align: 'center',
      iconSize: '40px',
      iconHeight: '40px',
      innerPadding: '8px',
      paddingTop: '20px',
      paddingRight: '25px',
      paddingBottom: '20px',
      paddingLeft: '25px'
    }

    // Insert content into social column
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'social-column-1', socialTitle, 0)!
    emailTree = EmailBlockClass.insertBlockIntoTree(emailTree, 'social-column-1', socialBlock, 1)!

    return emailTree
  }

  /**
   * Static utility method to redistribute column widths equally within an mj-group
   * This ensures all columns in a group have equal widths that sum to 100%
   */
  static redistributeGroupColumnWidths(tree: EmailBlock, groupId: string): EmailBlock {
    // Create a deep copy to avoid mutation
    const updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    // Find the group block
    const groupBlock = EmailBlockClass.findBlockById(updatedTree, groupId)
    if (!groupBlock || groupBlock.type !== 'mj-group' || !groupBlock.children) {
      return updatedTree
    }

    // Filter only mj-column children
    const columns = groupBlock.children.filter((child) => child.type === 'mj-column')
    const columnCount = columns.length

    if (columnCount === 0) {
      return updatedTree
    }

    // Calculate equal width for all columns
    const equalWidth = `${100 / columnCount}%`

    // Update all columns with equal widths
    columns.forEach((column) => {
      if (!column.attributes) {
        column.attributes = {}
      }
      column.attributes.width = equalWidth
    })

    console.log(
      `Redistributed widths for ${columnCount} columns in group ${groupId}: ${equalWidth} each`
    )
    return updatedTree
  }

  /**
   * Static utility method to redistribute column widths in both source and target containers
   * when a column is moved between different containers (sections or groups)
   */
  static redistributeColumnWidthsAfterMove(
    tree: EmailBlock,
    movedBlockId: string,
    sourceParentId: string,
    targetParentId: string
  ): EmailBlock {
    let updatedTree = JSON.parse(JSON.stringify(tree)) as EmailBlock

    // Get the moved block to check if it's a column
    const movedBlock = EmailBlockClass.findBlockById(updatedTree, movedBlockId)
    if (!movedBlock || movedBlock.type !== 'mj-column') {
      return updatedTree
    }

    // Redistribute widths in source container if it's different from target
    if (sourceParentId !== targetParentId) {
      const sourceParent = EmailBlockClass.findBlockById(updatedTree, sourceParentId)
      if (sourceParent) {
        if (sourceParent.type === 'mj-section') {
          const sourceColumns =
            sourceParent.children?.filter((child) => child.type === 'mj-column') || []
          if (sourceColumns.length > 0) {
            const equalWidth = `${100 / sourceColumns.length}%`
            sourceColumns.forEach((column) => {
              if (!column.attributes) column.attributes = {}
              column.attributes.width = equalWidth
            })
          }
        } else if (sourceParent.type === 'mj-group') {
          updatedTree = EmailBlockClass.redistributeGroupColumnWidths(updatedTree, sourceParentId)
        }
      }
    }

    // Redistribute widths in target container
    const targetParent = EmailBlockClass.findBlockById(updatedTree, targetParentId)
    if (targetParent) {
      if (targetParent.type === 'mj-section') {
        const targetColumns =
          targetParent.children?.filter((child) => child.type === 'mj-column') || []
        if (targetColumns.length > 0) {
          const equalWidth = `${100 / targetColumns.length}%`
          targetColumns.forEach((column) => {
            if (!column.attributes) column.attributes = {}
            column.attributes.width = equalWidth
          })
        }
      } else if (targetParent.type === 'mj-group') {
        updatedTree = EmailBlockClass.redistributeGroupColumnWidths(updatedTree, targetParentId)
      }
    }

    return updatedTree
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
