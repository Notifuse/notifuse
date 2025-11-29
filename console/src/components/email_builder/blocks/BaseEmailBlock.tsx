import React from 'react'
import type { EmailBlock, MJMLComponentType, SaveOperation, SavedBlock, MergedBlockAttributes } from '../types'

// Define the onUpdate function type - accepts object of attributes to update
export type OnUpdateAttributesFunction = (updates: Record<string, unknown>) => void

export interface PreviewProps {
  selectedBlockId: string | null
  onSelectBlock: (blockId: string) => void
  onUpdateBlock: (blockId: string, updates: EmailBlock) => void
  onCloneBlock: (blockId: string) => void
  onDeleteBlock: (blockId: string) => void
  attributeDefaults: Partial<Record<MJMLComponentType, Record<string, unknown>>>
  emailTree: EmailBlock
  onSaveBlock: (block: EmailBlock, operation: SaveOperation, nameOrId: string) => void
  savedBlocks?: SavedBlock[]
}

/**
 * Abstract base class for all email block types
 * Each block type should extend this class and implement its specific behavior
 */
export abstract class BaseEmailBlock {
  protected block: EmailBlock

  constructor(block: EmailBlock) {
    this.block = block
  }

  /**
   * Get the FontAwesome icon for this block type
   */
  abstract getIcon(parentType?: MJMLComponentType): React.ReactNode

  /**
   * Get human-readable label for this block
   */
  abstract getLabel(index?: number): string

  /**
   * Get description for this block type
   */
  abstract getDescription(): React.ReactNode

  /**
   * Get category for this block type
   */
  abstract getCategory(): 'content' | 'layout'

  /**
   * Get a visual illustration for this block type (optional)
   */
  getIllustration?(): React.ReactNode

  /**
   * Get default attributes for this block type
   */
  abstract getDefaults(): Record<string, unknown>

  /**
   * Check if this block can contain children
   */
  abstract canHaveChildren(): boolean

  /**
   * Get valid child component types for this block
   */
  abstract getValidChildTypes(): MJMLComponentType[]

  /**
   * Check if a component type can be dropped into this block
   */
  canAcceptChild(childType: MJMLComponentType): boolean {
    return this.getValidChildTypes().includes(childType)
  }

  /**
   * Get the underlying block data
   */
  getBlock(): EmailBlock {
    return this.block
  }

  /**
   * Get the block type
   */
  getType(): MJMLComponentType {
    return this.block.type
  }

  /**
   * Get the block ID
   */
  getId(): string {
    return this.block.id
  }

  /**
   * Render the block's settings panel content
   * This must be implemented by all block types
   */
  abstract renderSettingsPanel(
    onUpdateAttributes: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode

  /**
   * Generate the preview representation of this block
   */
  abstract getEdit(props: PreviewProps): React.ReactNode
}
