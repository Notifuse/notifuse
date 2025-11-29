import React from 'react'
import { InputNumber } from 'antd'
import type { MJMLComponentType, MJSpacerAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import PaddingInput from '../ui/PaddingInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import PanelLayout from '../panels/PanelLayout'

/**
 * Implementation for mj-spacer blocks
 */
export class MjSpacerBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="svg-inline--fa"
      >
        <path d="M12 22v-6" />
        <path d="M12 8V2" />
        <path d="M4 12H2" />
        <path d="M10 12H8" />
        <path d="M16 12h-2" />
        <path d="M22 12h-2" />
        <path d="m15 19-3 3-3-3" />
        <path d="m15 5-3-3-3 3" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Spacer'
  }

  getDescription(): React.ReactNode {
    return 'Vertical spacing component to add space between content elements'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-spacer'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the spacer block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSpacerAttributes

    return (
      <PanelLayout title="Spacer Attributes">
        <InputLayout label="Height">
          <InputNumber
            size="small"
            value={this.parsePixelValue(currentAttributes.height)}
            onChange={(value) => onUpdate({ height: value ? `${value}px` : undefined })}
            placeholder={(this.parsePixelValue(blockDefaults.height) || 20).toString()}
            min={0}
            max={500}
            step={1}
            suffix="px"
            style={{ width: '100%' }}
          />
        </InputLayout>

        <InputLayout label="Container Background">
          <ColorPickerWithPresets
            value={currentAttributes.containerBackgroundColor || undefined}
            onChange={(color) => onUpdate({ containerBackgroundColor: color || undefined })}
            placeholder="Transparent"
          />
        </InputLayout>

        <InputLayout label="Padding" layout="vertical">
          <PaddingInput
            value={{
              top: currentAttributes.paddingTop,
              right: currentAttributes.paddingRight,
              bottom: currentAttributes.paddingBottom,
              left: currentAttributes.paddingLeft
            }}
            defaultValue={{
              top: blockDefaults.paddingTop,
              right: blockDefaults.paddingRight,
              bottom: blockDefaults.paddingBottom,
              left: blockDefaults.paddingLeft
            }}
            onChange={(values: {
              top: string | undefined
              right: string | undefined
              bottom: string | undefined
              left: string | undefined
            }) => {
              onUpdate({
                paddingTop: values.top,
                paddingRight: values.right,
                paddingBottom: values.bottom,
                paddingLeft: values.left
              })
            }}
          />
        </InputLayout>

        <InputLayout label="CSS Class">
          <StringPopoverInput
            value={currentAttributes.cssClass || ''}
            onChange={(value) => onUpdate({ cssClass: value || undefined })}
            placeholder="Enter CSS class name"
          />
        </InputLayout>
      </PanelLayout>
    )
  }

  /**
   * Parse pixel value from string (e.g., "20px" -> 20)
   */
  private parsePixelValue(value?: string): number | undefined {
    if (!value) return undefined
    const match = value.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      attributeDefaults
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
      'mj-spacer',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Container style (equivalent to the td wrapper in MJML)
    const containerStyle: React.CSSProperties = {
      padding: `${attrs.paddingTop || '0px'} ${attrs.paddingRight || '0px'} ${
        attrs.paddingBottom || '0px'
      } ${attrs.paddingLeft || '0px'}`,
      backgroundColor: attrs.containerBackgroundColor,
      fontSize: '0px', // MJML sets font-size to 0 on container
      lineHeight: '0px',
      ...selectionStyle
    }

    // Spacer content style - creates the vertical space
    const spacerStyle: React.CSSProperties = {
      display: 'block',
      width: '100%',
      height: attrs.height || '20px',
      fontSize: '1px', // Prevent collapsing in some email clients
      lineHeight: '1px',
      backgroundColor: 'transparent' // Spacer itself should be invisible
    }

    return (
      <div
        key={key}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        style={{ ...containerStyle, position: 'relative' }}
        data-block-id={this.block.id}
      >
        <div style={spacerStyle}>&nbsp;</div>
      </div>
    )
  }
}
