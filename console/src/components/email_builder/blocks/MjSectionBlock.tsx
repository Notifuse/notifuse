import React from 'react'
import { Switch, Radio, Tooltip } from 'antd'
import type { MJMLComponentType, EmailBlock, MJSectionAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import BackgroundInput from '../ui/BackgroundInput'
import BorderInput from '../ui/BorderInput'
import PaddingInput from '../ui/PaddingInput'
import AlignSelector from '../ui/AlignSelector'
import StringPopoverInput from '../ui/StringPopoverInput'
import BorderRadiusInput from '../ui/BorderRadiusInput'

/**
 * Implementation for mj-section blocks
 */
export class MjSectionBlock extends BaseEmailBlock {
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
        <rect width="20" height="12" x="2" y="6" rx="2" />
        <path d="M12 12h.01" />
        <path d="M17 12h.01" />
        <path d="M7 12h.01" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Section'
  }

  getDescription(): React.ReactNode {
    return 'Container for columns that organizes email layout horizontally'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, any> {
    return MJML_COMPONENT_DEFAULTS['mj-section'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-column', 'mj-group', 'mj-raw']
  }

  /**
   * Render the settings panel for the section block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: Record<string, any>,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSectionAttributes

    const handleAttributeChange = (key: string, value: any) => {
      onUpdate({ [key]: value })
    }

    const handleBackgroundChange = (backgroundValues: any) => {
      onUpdate(backgroundValues)
    }

    return (
      <PanelLayout title="Section Attributes">
        <div className="space-y-4">
          {/* Background Settings */}
          <BackgroundInput
            value={{
              backgroundColor: currentAttributes.backgroundColor,
              backgroundUrl: currentAttributes.backgroundUrl,
              backgroundSize: currentAttributes.backgroundSize,
              backgroundRepeat: currentAttributes.backgroundRepeat,
              backgroundPosition: currentAttributes.backgroundPosition,
              backgroundPositionX: currentAttributes.backgroundPositionX,
              backgroundPositionY: currentAttributes.backgroundPositionY
            }}
            onChange={handleBackgroundChange}
          />

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

          {/* Layout Settings */}
          <InputLayout label="Text Alignment">
            <AlignSelector
              value={currentAttributes.textAlign || blockDefaults.textAlign || 'left'}
              onChange={(value) => handleAttributeChange('textAlign', value)}
            />
          </InputLayout>

          <InputLayout label="Text Direction">
            <Radio.Group
              size="small"
              value={currentAttributes.direction || blockDefaults.direction || 'ltr'}
              onChange={(e) => handleAttributeChange('direction', e.target.value)}
            >
              <Radio.Button value="ltr">
                <Tooltip title="Left to Right">LTR</Tooltip>
              </Radio.Button>
              <Radio.Button value="rtl">
                <Tooltip title="Right to Left">RTL</Tooltip>
              </Radio.Button>
            </Radio.Group>
          </InputLayout>

          <InputLayout
            label="Full Width"
            help="Makes the section span the entire email viewport width, ignoring container constraints (typically 600px). Useful for full-bleed backgrounds and hero sections."
          >
            <Switch
              size="small"
              checked={currentAttributes.fullWidth === 'full-width'}
              onChange={(checked) =>
                handleAttributeChange('fullWidth', checked ? 'full-width' : undefined)
              }
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
      'mj-section',
      this.block.attributes,
      attributeDefaults
    )

    // MJML wrapper div style - handles background, max-width, and centering
    const wrapperStyle: React.CSSProperties = {
      backgroundColor: attrs.backgroundColor,
      backgroundImage: attrs.backgroundUrl ? `url(${attrs.backgroundUrl})` : undefined,
      backgroundRepeat: attrs.backgroundRepeat,
      backgroundSize: attrs.backgroundSize,
      // Handle background position properly - prioritize combined position over individual X/Y
      ...(attrs.backgroundPosition
        ? { backgroundPosition: attrs.backgroundPosition }
        : {
            ...(attrs.backgroundPositionX && { backgroundPositionX: attrs.backgroundPositionX }),
            ...(attrs.backgroundPositionY && { backgroundPositionY: attrs.backgroundPositionY })
          }),
      margin: '0px auto',
      maxWidth: attrs.fullWidth === 'full-width' ? undefined : '600px',
      borderRadius: attrs.borderRadius !== '0px' ? attrs.borderRadius : undefined,
      ...selectionStyle
    }

    // MJML table style - duplicates background for email client compatibility
    const tableStyle: React.CSSProperties = {
      backgroundColor: attrs.backgroundColor,
      width: '100%',
      borderTop: attrs.borderTop !== 'none' ? attrs.borderTop : undefined,
      borderRight: attrs.borderRight !== 'none' ? attrs.borderRight : undefined,
      borderBottom: attrs.borderBottom !== 'none' ? attrs.borderBottom : undefined,
      borderLeft: attrs.borderLeft !== 'none' ? attrs.borderLeft : undefined
    }

    // MJML td style - handles padding, direction, text-align
    const cellStyle: React.CSSProperties = {
      direction: attrs.direction as any,
      fontSize: '0px', // MJML sets this to 0 to prevent spacing issues
      padding: `${attrs.paddingTop || '20px'} ${attrs.paddingRight || '0'} ${
        attrs.paddingBottom || '20px'
      } ${attrs.paddingLeft || '0'}`,
      textAlign: attrs.textAlign as any
    }

    // Check if section has no columns or groups
    const hasContent =
      this.block.children &&
      this.block.children.some((child) => child.type === 'mj-column' || child.type === 'mj-group')

    const contentElement = !hasContent ? (
      <div
        style={{
          padding: '20px',
          backgroundColor: '#fff3cd',
          border: '1px solid #ffeaa7',
          borderRadius: '4px',
          color: '#856404',
          fontSize: '14px',
          textAlign: 'center',
          margin: '10px'
        }}
      >
        ⚠️ This section is empty. Add columns or groups to display content.
      </div>
    ) : (
      // Wrap columns in a table row structure as MJML does for multiple columns
      <table
        role="presentation"
        border={0}
        cellPadding="0"
        cellSpacing="0"
        style={{ width: '100%' }}
      >
        <tbody>
          <tr>
            {this.block.children?.map((child) => (
              <td key={child.id} style={{ verticalAlign: 'top' }}>
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
              </td>
            ))}
          </tr>
        </tbody>
      </table>
    )

    // Simulate MJML's exact structure: wrapper div > table > tbody > tr > td > columns
    return (
      <div
        key={key}
        style={{ ...wrapperStyle, position: 'relative' }}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        <table
          align="center"
          border={0}
          cellPadding="0"
          cellSpacing="0"
          role="presentation"
          style={tableStyle}
        >
          <tbody>
            <tr>
              <td style={cellStyle}>{contentElement}</td>
            </tr>
          </tbody>
        </table>
      </div>
    )
  }
}
