import React from 'react'
import { Radio, Tooltip } from 'antd'
import type { MJMLComponentType, EmailBlock, MJColumnAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import BorderInput from '../ui/BorderInput'
import PaddingInput from '../ui/PaddingInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import BorderRadiusInput from '../ui/BorderRadiusInput'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'

/**
 * Implementation for mj-column blocks
 */
export class MjColumnBlock extends BaseEmailBlock {
  getIcon(parentType?: MJMLComponentType): React.ReactNode {
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
        <rect width="18" height="18" x="3" y="3" rx="2" />
        <path d="M12 3v18" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Column'
  }

  getDescription(): React.ReactNode {
    return 'Vertical container within sections for organizing content in columns'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-column'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-text', 'mj-image', 'mj-button', 'mj-divider', 'mj-spacer', 'mj-social', 'mj-raw']
  }

  /**
   * Render the settings panel for the column block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, any>,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJColumnAttributes

    const handleAttributeChange = (key: string, value: any) => {
      onUpdate({ [key]: value })
    }

    return (
      <PanelLayout title="Column Attributes">
        <div className="space-y-4">
          {/* Layout Settings */}
          <InputLayout label="Width" help="Column width (e.g., '50%', '300px', 'auto')">
            <StringPopoverInput
              value={currentAttributes.width}
              onChange={(value) => handleAttributeChange('width', value)}
              placeholder={blockDefaults.width || '100%'}
              buttonText="Set Width"
            />
          </InputLayout>

          <InputLayout label="Vertical Alignment">
            <Radio.Group
              size="small"
              value={currentAttributes.verticalAlign || blockDefaults.verticalAlign || 'top'}
              onChange={(e) => handleAttributeChange('verticalAlign', e.target.value)}
            >
              <Radio.Button value="top">
                <Tooltip title="Align to top">Top</Tooltip>
              </Radio.Button>
              <Radio.Button value="middle">
                <Tooltip title="Align to middle">Middle</Tooltip>
              </Radio.Button>
              <Radio.Button value="bottom">
                <Tooltip title="Align to bottom">Bottom</Tooltip>
              </Radio.Button>
            </Radio.Group>
          </InputLayout>

          <InputLayout label="Background color">
            <ColorPickerWithPresets
              value={currentAttributes.backgroundColor || undefined}
              onChange={(color) => onUpdate({ backgroundColor: color || undefined })}
            />
          </InputLayout>
          <InputLayout label="Inner background color" help="Requires a padding">
            <ColorPickerWithPresets
              value={currentAttributes.innerBackgroundColor || undefined}
              onChange={(color) => onUpdate({ innerBackgroundColor: color || undefined })}
              placeholder="None"
            />
          </InputLayout>

          {/* Border Settings */}
          <InputLayout label="Border" layout="vertical">
            <BorderInput
              className="-mt-6"
              value={{
                borderTop: currentAttributes.borderTop,
                borderRight: currentAttributes.borderRight,
                borderBottom: currentAttributes.borderBottom,
                borderLeft: currentAttributes.borderLeft
              }}
              onChange={(borderValues) => {
                onUpdate({
                  borderTop: borderValues.borderTop,
                  borderRight: borderValues.borderRight,
                  borderBottom: borderValues.borderBottom,
                  borderLeft: borderValues.borderLeft
                })
              }}
            />
          </InputLayout>

          {/* Border Radius */}
          <InputLayout label="Border radius">
            <BorderRadiusInput
              value={currentAttributes.borderRadius}
              onChange={(value) => onUpdate({ borderRadius: value })}
              defaultValue={blockDefaults.borderRadius}
            />
          </InputLayout>

          {/* Inner Border Settings */}
          <InputLayout label="Inner border" layout="vertical">
            <BorderInput
              className="-mt-6"
              value={{
                borderTop: currentAttributes.innerBorderTop,
                borderRight: currentAttributes.innerBorderRight,
                borderBottom: currentAttributes.innerBorderBottom,
                borderLeft: currentAttributes.innerBorderLeft
              }}
              onChange={(borderValues) => {
                onUpdate({
                  innerBorderTop: borderValues.borderTop,
                  innerBorderRight: borderValues.borderRight,
                  innerBorderBottom: borderValues.borderBottom,
                  innerBorderLeft: borderValues.borderLeft
                })
              }}
            />
          </InputLayout>

          {/* Border Radius */}
          <InputLayout label="Inner border radius">
            <BorderRadiusInput
              value={currentAttributes.innerBorderRadius}
              onChange={(value) => onUpdate({ innerBorderRadius: value })}
              defaultValue={blockDefaults.innerBorderRadius}
            />
          </InputLayout>

          {/* Padding Settings */}
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

          {/* Advanced Settings */}
          <InputLayout label="CSS Class" help="Custom CSS class for styling">
            <StringPopoverInput
              value={currentAttributes.cssClass}
              onChange={(value) => handleAttributeChange('cssClass', value)}
              placeholder="my-custom-class"
              buttonText="Set Value"
            />
          </InputLayout>
        </div>
      </PanelLayout>
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      onCloneBlock,
      onDeleteBlock,
      attributeDefaults,
      emailTree,
      onUpdateBlock,
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
      'mj-column',
      this.block.attributes,
      attributeDefaults
    )

    // Calculate width percentage for responsive classes
    const widthPercent =
      attrs.width && attrs.width.includes('%') ? attrs.width.replace('%', '') : '100'

    // MJML generates a wrapper div with specific classes and styles
    const columnWrapperStyle: React.CSSProperties = {
      fontSize: '0px',
      textAlign: 'left',
      direction: 'ltr',
      display: 'inline-block',
      verticalAlign: attrs.verticalAlign || 'top',
      width: attrs.width || '100%',
      ...selectionStyle
    }

    // Inner table style - matches MJML's presentation table
    const innerTableStyle: React.CSSProperties = {
      verticalAlign: 'top'
    }

    // Content cell style - where the actual content goes
    const contentCellStyle: React.CSSProperties = {
      fontSize: '0px',
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      wordBreak: 'break-word',
      backgroundColor: attrs.backgroundColor,
      borderTop: attrs.borderTop,
      borderRight: attrs.borderRight,
      borderBottom: attrs.borderBottom,
      borderLeft: attrs.borderLeft,
      borderRadius: attrs.borderRadius
    }

    // Inner styling container (for inner background/borders)
    const innerStylingStyle: React.CSSProperties = {
      backgroundColor: attrs.innerBackgroundColor,
      borderTop: attrs.innerBorderTop,
      borderRight: attrs.innerBorderRight,
      borderBottom: attrs.innerBorderBottom,
      borderLeft: attrs.innerBorderLeft,
      borderRadius: attrs.innerBorderRadius,
      fontSize: '13px' // Reset font size for content
    }

    // Check if column has content
    const hasContent = this.block.children && this.block.children.length > 0

    // Check if we need inner styling container
    const hasInnerStyling =
      attrs.innerBackgroundColor ||
      attrs.innerBorderTop ||
      attrs.innerBorderRight ||
      attrs.innerBorderBottom ||
      attrs.innerBorderLeft ||
      attrs.innerBorderRadius

    const contentElement = !hasContent ? (
      <div
        style={{
          padding: '15px',
          backgroundColor: '#f8f9fa',
          border: '2px dashed #dee2e6',
          borderRadius: '4px',
          color: '#6c757d',
          fontSize: '12px',
          textAlign: 'center',
          margin: '5px'
        }}
      >
        📝 Empty column. Add text, button, or image content.
      </div>
    ) : (
      this.block.children?.map((child) => (
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
      ))
    )

    // Simulate MJML's exact structure: div wrapper > table > tbody > tr > td
    return (
      <div
        key={key}
        className={`mj-column-per-${widthPercent} mj-outlook-group-fix ${
          attrs.cssClass || ''
        } ${blockClasses}`.trim()}
        style={{ ...columnWrapperStyle, position: 'relative' }}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        <table
          border={0}
          cellPadding="0"
          cellSpacing="0"
          role="presentation"
          style={innerTableStyle}
          width="100%"
        >
          <tbody>
            <tr>
              <td align="left" style={contentCellStyle}>
                {hasInnerStyling ? (
                  <div style={innerStylingStyle}>{contentElement}</div>
                ) : (
                  <div style={{ fontSize: '13px' }}>{contentElement}</div>
                )}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    )
  }
}
