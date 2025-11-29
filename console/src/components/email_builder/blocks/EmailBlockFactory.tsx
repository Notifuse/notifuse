import type { EmailBlock, MJMLComponentType } from '../types'
import { BaseEmailBlock } from './BaseEmailBlock'
import { MjmlBlock } from './MjmlBlock'
import { MjBodyBlock } from './MjBodyBlock'
import { MjWrapperBlock } from './MjWrapperBlock'
import { MjTextBlock } from './MjTextBlock'
import { MjButtonBlock } from './MjButtonBlock'
import { MjImageBlock } from './MjImageBlock'
import { MjDividerBlock } from './MjDividerBlock'
import { MjSpacerBlock } from './MjSpacerBlock'
import { MjSocialBlock } from './MjSocialBlock'
import { MjSectionBlock } from './MjSectionBlock'
import { MjColumnBlock } from './MjColumnBlock'
import { MjGroupBlock } from './MjGroupBlock'
import { MjBreakpointBlock } from './MjBreakpointBlock'
import { MjFontBlock } from './MjFontBlock'
import { MjStyleBlock } from './MjStyleBlock'
import { MjPreviewBlock } from './MjPreviewBlock'
import { MjTitleBlock } from './MjTitleBlock'
import { MjHeadBlock } from './MjHeadBlock'
import { MjAttributesBlock } from './MjAttributesBlock'
import { MjRawBlock } from './MjRawBlock'
import { MjSocialElementBlock } from './MjSocialElementBlock'
// Import other block types as they're created
// import { MjImageBlock } from './MjImageBlock'
// etc...

/**
 * Factory class for creating the appropriate block class instance
 * based on the block type
 */
export class EmailBlockFactory {
  private static blockRegistry: Map<MJMLComponentType, new (block: EmailBlock) => BaseEmailBlock> =
    new Map([
      ['mjml', MjmlBlock],
      ['mj-body', MjBodyBlock],
      ['mj-wrapper', MjWrapperBlock],
      ['mj-section', MjSectionBlock],
      ['mj-column', MjColumnBlock],
      ['mj-group', MjGroupBlock],
      ['mj-text', MjTextBlock],
      ['mj-button', MjButtonBlock],
      ['mj-image', MjImageBlock],
      ['mj-divider', MjDividerBlock],
      ['mj-spacer', MjSpacerBlock],
      ['mj-social', MjSocialBlock],
      ['mj-head', MjHeadBlock],
      ['mj-attributes', MjAttributesBlock],
      ['mj-breakpoint', MjBreakpointBlock],
      ['mj-font', MjFontBlock],
      ['mj-style', MjStyleBlock],
      ['mj-preview', MjPreviewBlock],
      ['mj-title', MjTitleBlock],
      ['mj-raw', MjRawBlock],
      ['mj-social-element', MjSocialElementBlock]
    ])

  /**
   * Create the appropriate block class instance for the given block
   */
  static createBlock(block: EmailBlock): BaseEmailBlock {
    const BlockClass = this.blockRegistry.get(block.type)

    if (!BlockClass) {
      // Fallback to a generic implementation or throw an error
      throw new Error(`No block class registered for type: ${block.type}`)
    }

    return new BlockClass(block)
  }

  /**
   * Check if a block type has a registered implementation
   */
  static hasBlockType(type: MJMLComponentType): boolean {
    return this.blockRegistry.has(type)
  }

  /**
   * Get all registered block types
   */
  static getRegisteredTypes(): MJMLComponentType[] {
    return Array.from(this.blockRegistry.keys())
  }

  /**
   * Get block metadata for all registered types that can be children of the given parent type
   */
  static getAvailableBlocksForParent(parentType: MJMLComponentType): Array<{
    type: MJMLComponentType
    label: string
    description: React.ReactNode
    category: 'content' | 'layout'
    icon: React.ReactNode
  }> {
    // Create a temporary parent block to get valid child types
    const parentBlock = this.createBlock({
      id: 'temp',
      type: parentType,
      attributes: {}
    } as EmailBlock)

    const validChildTypes = parentBlock.getValidChildTypes()

    return validChildTypes
      .filter((type) => this.hasBlockType(type))
      .map((type) => {
        // Create a temporary block instance to get metadata
        const tempBlock = this.createBlock({
          id: 'temp',
          type,
          attributes: {}
        } as EmailBlock)

        return {
          type,
          label: tempBlock.getLabel(),
          description: tempBlock.getDescription(),
          category: tempBlock.getCategory(),
          icon: tempBlock.getIcon()
        }
      })
  }

  /**
   * Register a new block type (useful for plugins or custom blocks)
   */
  static registerBlockType(
    type: MJMLComponentType,
    blockClass: new (block: EmailBlock) => BaseEmailBlock
  ): void {
    this.blockRegistry.set(type, blockClass)
  }

  /**
   * Unregister a block type
   */
  static unregisterBlockType(type: MJMLComponentType): void {
    this.blockRegistry.delete(type)
  }
}
