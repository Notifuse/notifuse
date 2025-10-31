import React from 'react'
import { Alert } from 'antd'
import type { MJMLComponentType, EmailBlock, MJBodyAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import StringPopoverInput from '../ui/StringPopoverInput'
import WidthPxInput from '../ui/WidthPxInput'

/**
 * Implementation for mj-body blocks
 */
export class MjBodyBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return null
  }

  getLabel(): string {
    return 'Body'
  }

  getDescription(): React.ReactNode {
    return 'Main container for email content, contains sections'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-body'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-wrapper', 'mj-section', 'mj-raw']
  }

  /**
   * Render the settings panel for the body block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, any>,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJBodyAttributes

    // Parse width to check if it exceeds 650px
    const parseWidth = (width?: string): number | undefined => {
      if (!width) return undefined
      const match = width.match(/^(\d+(?:\.\d+)?)px?$/)
      return match ? parseFloat(match[1]) : undefined
    }

    const widthValue = parseWidth(currentAttributes.width || blockDefaults.width)
    const showWarning = widthValue !== undefined && widthValue > 650

    return (
      <PanelLayout title="Body Attributes">
        <InputLayout label="Width">
          <WidthPxInput
            value={currentAttributes.width}
            onChange={(value) => onUpdate({ width: value })}
            placeholder={blockDefaults.width || '600px'}
            max={999}
          />
        </InputLayout>

        {showWarning && (
          <Alert
            message="Email widths above 650px may not display correctly in some email clients"
            type="warning"
            style={{ marginTop: '8px' }}
          />
        )}
        <InputLayout label="Background Color">
          <ColorPickerWithPresets
            value={currentAttributes.backgroundColor || undefined}
            onChange={(color) => {
              onUpdate({ backgroundColor: color || undefined })
            }}
            placeholder="None"
          />
        </InputLayout>

        <InputLayout label="CSS Class">
          <StringPopoverInput
            value={currentAttributes.cssClass || ''}
            onChange={(value) => onUpdate({ cssClass: value || undefined })}
            placeholder="Enter CSS class name"
            buttonText="Set class"
          />
        </InputLayout>
      </PanelLayout>
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      attributeDefaults,
      emailTree,
      onUpdateBlock,
      onCloneBlock,
      onDeleteBlock,
      onSaveBlock: onSave,
      savedBlocks
    } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected
      ? { position: 'relative', zIndex: 10 }
      : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-body',
      this.block.attributes,
      attributeDefaults
    )

    const bodyStyle: React.CSSProperties = {
      width: attrs.width,
      backgroundColor: attrs.backgroundColor,
      ...selectionStyle
    }

    return (
      <div
        key={key}
        style={bodyStyle}
        className={`${attrs.cssClass} ${blockClasses}`.trim()}
        onClick={handleClick}
      >
        {this.block.children?.map((child) => (
          <React.Fragment key={child.id}>
            {EmailBlockClass.renderEmailBlock(
              child,
              attributeDefaults,
              selectedBlockId,
              onSelectBlock,
              emailTree,
              onUpdateBlock,
              onCloneBlock,
              onDeleteBlock,
              onSave,
              savedBlocks
            )}
          </React.Fragment>
        ))}
      </div>
    )
  }
}
