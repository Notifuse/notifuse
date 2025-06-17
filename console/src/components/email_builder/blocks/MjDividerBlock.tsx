import React from 'react'
import { Select, InputNumber, Row, Col } from 'antd'
import type { MJMLComponentType, EmailBlock, MJDividerAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import AlignSelector from '../ui/AlignSelector'
import PaddingInput from '../ui/PaddingInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import PanelLayout from '../panels/PanelLayout'
import WidthInput from '../ui/WidthInput'

/**
 * Implementation for mj-divider blocks
 */
export class MjDividerBlock extends BaseEmailBlock {
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
        <line x1="3" y1="12" x2="21" y2="12" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Divider'
  }

  getDescription(): React.ReactNode {
    return 'Horizontal divider line to separate content sections'
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-divider'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the divider block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, any>,
    _emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJDividerAttributes

    return (
      <PanelLayout title="Divider Attributes">
        <InputLayout label="Border Style" layout="vertical">
          <Row gutter={16}>
            <Col span={8}>
              <div className="mb-2">
                <span className="text-xs text-gray-500">Color</span>
                <div style={{ marginTop: '4px' }}>
                  <ColorPickerWithPresets
                    value={currentAttributes.borderColor || undefined}
                    onChange={(color) => onUpdate({ borderColor: color || undefined })}
                  />
                </div>
              </div>
            </Col>
            <Col span={8}>
              <div className="mb-2">
                <span className="text-xs text-gray-500">Style</span>
                <div style={{ marginTop: '4px' }}>
                  <Select
                    size="small"
                    value={currentAttributes.borderStyle || 'solid'}
                    onChange={(value) => onUpdate({ borderStyle: value })}
                    style={{ width: '100%' }}
                  >
                    <Select.Option value="solid">Solid</Select.Option>
                    <Select.Option value="dashed">Dashed</Select.Option>
                    <Select.Option value="dotted">Dotted</Select.Option>
                  </Select>
                </div>
              </div>
            </Col>
            <Col span={8}>
              <div className="mb-2">
                <span className="text-xs text-gray-500">Width</span>
                <div style={{ marginTop: '4px' }}>
                  <InputNumber
                    size="small"
                    value={this.parsePixelValue(currentAttributes.borderWidth)}
                    onChange={(value) =>
                      onUpdate({ borderWidth: value ? `${value}px` : undefined })
                    }
                    placeholder={(this.parsePixelValue(blockDefaults.borderWidth) || 4).toString()}
                    min={0}
                    max={50}
                    step={1}
                    suffix="px"
                    style={{ width: '100%' }}
                  />
                </div>
              </div>
            </Col>
          </Row>
        </InputLayout>

        <InputLayout label="Divider Width">
          <WidthInput
            value={currentAttributes.width}
            onChange={(value) => onUpdate({ width: value })}
            placeholder={blockDefaults.width || 'Auto'}
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

        <InputLayout label="Alignment">
          <AlignSelector
            value={currentAttributes.align || 'center'}
            onChange={(value) => onUpdate({ align: value })}
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
   * Parse pixel value from string (e.g., "4px" -> 4)
   */
  private parsePixelValue(value?: string): number | undefined {
    if (!value) return undefined
    const match = value.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
  }

  /**
   * Parse width to get numeric value
   */
  private parseWidthNumber(width?: string): number | undefined {
    if (!width) return undefined
    const match = width.match(/^(\d+(?:\.\d+)?)px?$/)
    return match ? parseFloat(match[1]) : undefined
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const { selectedBlockId, onSelectBlock, attributeDefaults } = props

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
      'mj-divider',
      this.block.attributes,
      attributeDefaults
    )

    // Container style (equivalent to the td wrapper in MJML)
    const containerStyle: React.CSSProperties = {
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      backgroundColor: attrs.containerBackgroundColor,
      textAlign: (attrs.align as any) || 'center',
      fontSize: '0px', // MJML sets font-size to 0 on container
      ...selectionStyle
    }

    // Divider line style
    const dividerStyle: React.CSSProperties = {
      display: 'inline-block',
      width: attrs.width || '100%',
      height: '0px',
      border: 'none',
      borderTop: `${attrs.borderWidth || '4px'} ${attrs.borderStyle || 'solid'} ${
        attrs.borderColor || '#000000'
      }`,
      margin: '0px',
      fontSize: '1px', // Prevent collapsing in some email clients
      lineHeight: '1px'
    }

    return (
      <div key={key} style={containerStyle} className={blockClasses} onClick={handleClick}>
        <div style={dividerStyle}>&nbsp;</div>
      </div>
    )
  }
}
