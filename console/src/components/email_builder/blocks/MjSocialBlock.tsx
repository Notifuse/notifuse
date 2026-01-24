import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, MJSocialAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { MjSocialElementBlock } from './MjSocialElementBlock'
import InputLayout from '../ui/InputLayout'
import ColorPickerWithPresets from '../ui/ColorPickerWithPresets'
import PaddingInput from '../ui/PaddingInput'
import StringPopoverInput from '../ui/StringPopoverInput'
import PanelLayout from '../panels/PanelLayout'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faInstagram } from '@fortawesome/free-brands-svg-icons'
import FontStyleInput from '../ui/FontStyleInput'

// Functional component for settings panel with i18n support
interface MjSocialSettingsPanelProps {
  currentAttributes: MJSocialAttributes
  blockDefaults: MJSocialAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjSocialSettingsPanel: React.FC<MjSocialSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate
}) => {
  const { t } = useLingui()

  return (
    <PanelLayout title={t`Social Attributes`}>
      <InputLayout label={t`Container color`}>
        <ColorPickerWithPresets
          value={currentAttributes.containerBackgroundColor || undefined}
          onChange={(color) => onUpdate({ containerBackgroundColor: color || undefined })}
          placeholder={t`None`}
        />
      </InputLayout>

      <InputLayout label={t`Font Styling`} layout="vertical">
        <FontStyleInput
          value={{
            fontFamily: undefined,
            fontSize: undefined,
            fontWeight: undefined,
            fontStyle: undefined,
            textTransform: undefined,
            textDecoration: undefined,
            lineHeight: currentAttributes.lineHeight,
            letterSpacing: undefined,
            textAlign: currentAttributes.align
          }}
          defaultValue={{
            fontFamily: undefined,
            fontSize: undefined,
            fontWeight: undefined,
            fontStyle: undefined,
            textTransform: undefined,
            textDecoration: undefined,
            lineHeight: blockDefaults.lineHeight,
            letterSpacing: undefined,
            textAlign: blockDefaults.align
          }}
          onChange={(values) => {
            onUpdate({
              lineHeight: values.lineHeight,
              align: values.textAlign
            })
          }}
          importedFonts={[]}
        />
      </InputLayout>

      <InputLayout label={t`Padding`} layout="vertical">
        <PaddingInput
          value={currentAttributes.innerPadding}
          defaultValue={blockDefaults.innerPadding}
          onChange={(value: string | undefined) => {
            onUpdate({
              innerPadding: value
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`CSS Class`}>
        <StringPopoverInput
          value={currentAttributes.cssClass || ''}
          onChange={(value) => onUpdate({ cssClass: value || undefined })}
          placeholder={t`Enter CSS class name`}
        />
      </InputLayout>
    </PanelLayout>
  )
}

// Functional component for social placeholder with i18n support
const MjSocialPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
    <div
      style={{
        color: '#999',
        fontSize: '14px',
        fontFamily: 'Arial, sans-serif',
        padding: '20px',
        fontStyle: 'italic'
      }}
    >
      {t`Add social elements to this social block`}
    </div>
  )
}

/**
 * Implementation for mj-social blocks
 */
export class MjSocialBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faInstagram} />
  }

  getLabel(): string {
    return 'Social'
  }

  getDescription(): React.ReactNode {
    return 'Social media icons and links for connecting with your audience'
  }

  getCategory(): 'content' | 'layout' {
    return 'content'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-social'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-social-element']
  }

  /**
   * Render the settings panel for the social block
   */
  renderSettingsPanel(onUpdate: OnUpdateAttributesFunction): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSocialAttributes
    const blockDefaults = this.getDefaults() as MJSocialAttributes

    return (
      <MjSocialSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
      />
    )
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
      'mj-social',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Outer table style (simulates the main table wrapper)
    const outerTableStyle: React.CSSProperties = {
      width: '100%',
      border: '0',
      borderCollapse: 'collapse' as const,
      borderSpacing: '0',
      ...selectionStyle
    }

    // Container div style (simulates the margin auto container)
    const containerDivStyle: React.CSSProperties = {
      margin: '0px auto',
      maxWidth: '600px'
    }

    // Inner table style (main content table)
    const innerTableStyle: React.CSSProperties = {
      width: '100%',
      border: '0',
      borderCollapse: 'collapse' as const,
      borderSpacing: '0'
    }

    // Content cell style (the td that contains social elements)
    const contentCellStyle: React.CSSProperties = {
      direction: 'ltr' as const,
      fontSize: '0px',
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      textAlign: (attrs.align as 'left' | 'center' | 'right' | 'justify') || 'center',
      wordBreak: 'break-word'
    }

    // Check if we have children to render
    const hasChildren = this.block.children && this.block.children.length > 0

    // Content to render inside the table structure
    let socialContent: React.ReactNode

    if (hasChildren) {
      // Render actual children by instantiating MjSocialElementBlock instances
      const isVertical = attrs.mode === 'vertical'

      if (isVertical) {
        // Vertical mode: each child in its own row
        socialContent = (
          <div>
            {this.block.children!.map((child, index) => {
              const socialElementBlock = new MjSocialElementBlock(child)

              return (
                <div
                  key={child.id || index}
                  style={{
                    display: 'block',
                    marginBottom: index < this.block.children!.length - 1 ? '4px' : '0'
                  }}
                >
                  {socialElementBlock.getEdit(props)}
                </div>
              )
            })}
          </div>
        )
      } else {
        // Horizontal mode: children side by side
        socialContent = (
          <div style={{ display: 'inline-block' }}>
            {this.block.children!.map((child, index) => {
              const socialElementBlock = new MjSocialElementBlock(child)

              return (
                <React.Fragment key={child.id || index}>
                  {socialElementBlock.getEdit(props)}
                </React.Fragment>
              )
            })}
          </div>
        )
      }
    } else {
      // Show placeholder when no children exist (shouldn't happen normally since mj-social gets default children)
      socialContent = <MjSocialPlaceholder />
    }

    // Return the complete table structure that simulates MJML output
    return (
      <table
        key={key}
        align="center"
        border={0}
        cellPadding={0}
        cellSpacing={0}
        role="presentation"
        style={outerTableStyle}
        className={blockClasses}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        <tbody>
          <tr>
            <td style={{ lineHeight: '0px', fontSize: '0px' }}>
              <div style={containerDivStyle}>
                <table
                  align="center"
                  border={0}
                  cellPadding={0}
                  cellSpacing={0}
                  role="presentation"
                  style={innerTableStyle}
                >
                  <tbody>
                    <tr>
                      <td style={contentCellStyle}>{socialContent}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    )
  }
}
