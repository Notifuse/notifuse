import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, MJMLComponentType, MJSocialAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS, MJ_SOCIAL_ELEMENT_DEFAULTS, SOCIAL_INHERITED_ATTRIBUTES } from '../mjml-defaults'
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
import { SingleBorderInput } from '../ui/BorderInput'
import { Col, Input, InputNumber, Row, Segmented } from 'antd'
import BorderRadiusInput from '../ui/BorderRadiusInput'

// A px length as a number, for the number fields
const pixels = (value?: string): number | undefined => {
  const match = value?.match(/^(\d+(?:\.\d+)?)px$/)
  return match ? parseFloat(match[1]) : undefined
}

// mj-social has no text-transform nor letter-spacing attribute
const SOCIAL_HIDDEN_FONT_FIELDS = ['textTransform', 'letterSpacing'] as const

/**
 * What mj-social passes to each of its elements (see SOCIAL_INHERITED_ATTRIBUTES): the attributes the block or the
 * template's default mj-social attributes set, and its inner-padding as the elements' padding. MJML passes its own
 * defaults for them too, which are the elements' defaults: the canvas leaves those to the elements.
 */
export function socialInheritedAttributes(
  social: Record<string, unknown> | undefined,
  socialDefaults: Record<string, unknown> | undefined
): Record<string, unknown> {
  const set = { ...socialDefaults, ...social }
  const isSet = (value: unknown) => value !== undefined && value !== null && value !== ''
  const inherited: Record<string, unknown> = {}
  if (isSet(set.innerPadding)) inherited.padding = set.innerPadding
  for (const key of SOCIAL_INHERITED_ATTRIBUTES) {
    if (isSet(set[key])) inherited[key] = set[key]
  }
  return inherited
}

// Functional component for settings panel with i18n support
interface MjSocialSettingsPanelProps {
  currentAttributes: MJSocialAttributes
  blockDefaults: MJSocialAttributes
  onUpdate: OnUpdateAttributesFunction
  /** What an icon shows for an inherited attribute the block does not set */
  iconDefaults: Record<string, string | undefined>
  importedFonts: Array<{ name: string; href: string }>
}

const MjSocialSettingsPanel: React.FC<MjSocialSettingsPanelProps> = ({
  currentAttributes,
  blockDefaults,
  onUpdate,
  iconDefaults,
  importedFonts
}) => {
  const { t } = useLingui()
  // The layout the block takes without a mode of its own: the template's default mj-social mode, else a row
  const defaultMode = blockDefaults.mode === 'vertical' ? 'vertical' : 'horizontal'

  return (
    <PanelLayout title={t`Social attributes`}>
      <InputLayout label={t`Layout`}>
        <Segmented
          size="small"
          value={currentAttributes.mode ?? defaultMode}
          onChange={(value) =>
            onUpdate({ mode: value === defaultMode ? undefined : (value as MJSocialAttributes['mode']) })
          }
          options={[
            { label: t`In a row`, value: 'horizontal' },
            { label: t`In a column`, value: 'vertical' }
          ]}
        />
      </InputLayout>

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
            fontFamily: currentAttributes.fontFamily,
            fontSize: currentAttributes.fontSize,
            fontWeight: currentAttributes.fontWeight,
            fontStyle: currentAttributes.fontStyle,
            textTransform: undefined,
            textDecoration: currentAttributes.textDecoration,
            lineHeight: currentAttributes.lineHeight,
            letterSpacing: undefined,
            textAlign: currentAttributes.align
          }}
          defaultValue={{
            // What the icons show when mj-social passes nothing
            fontFamily: iconDefaults.fontFamily,
            fontSize: iconDefaults.fontSize,
            fontWeight: iconDefaults.fontWeight,
            fontStyle: iconDefaults.fontStyle,
            textTransform: undefined,
            textDecoration: iconDefaults.textDecoration,
            lineHeight: blockDefaults.lineHeight,
            letterSpacing: undefined,
            textAlign: blockDefaults.align
          }}
          onChange={(values) => {
            onUpdate({
              fontFamily: values.fontFamily,
              fontSize: values.fontSize,
              fontWeight: values.fontWeight,
              fontStyle: values.fontStyle,
              textDecoration: values.textDecoration,
              lineHeight: values.lineHeight,
              align: values.textAlign as MJSocialAttributes['align']
            })
          }}
          importedFonts={importedFonts}
          hiddenFields={SOCIAL_HIDDEN_FONT_FIELDS}
        />
      </InputLayout>

      <InputLayout
        label={t`Icon border`}
        help={t`Applies to every icon; an icon's own border wins.`}
        layout="vertical"
      >
        <SingleBorderInput
          value={currentAttributes.border}
          onChange={(value) => onUpdate({ border: value })}
        />
      </InputLayout>

      <InputLayout
        label={t`Icons`}
        help={t`Applies to every icon; an icon's own settings win.`}
        layout="vertical"
      >
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Size`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={pixels(currentAttributes.iconSize)}
                  onChange={(value) => onUpdate({ iconSize: value ? `${value}px` : undefined })}
                  placeholder="20"
                  min={10}
                  max={100}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Height`}</span>
              <div style={{ marginTop: '4px' }}>
                <InputNumber
                  size="small"
                  value={pixels(currentAttributes.iconHeight)}
                  onChange={(value) => onUpdate({ iconHeight: value ? `${value}px` : undefined })}
                  // Without a height, the icons are as high as their size
                  placeholder={String(pixels(currentAttributes.iconSize) ?? 20)}
                  min={10}
                  max={100}
                  step={1}
                  suffix="px"
                  style={{ width: '100%' }}
                />
              </div>
            </div>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Border Radius`}</span>
              <div style={{ marginTop: '4px' }}>
                <BorderRadiusInput
                  value={currentAttributes.borderRadius}
                  onChange={(value) => onUpdate({ borderRadius: value })}
                  defaultValue={blockDefaults.borderRadius}
                />
              </div>
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text color`}</span>
              <div style={{ marginTop: '4px' }}>
                <ColorPickerWithPresets
                  value={currentAttributes.color || undefined}
                  onChange={(color) => onUpdate({ color: color || undefined })}
                />
              </div>
            </div>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Icon padding`}</span>
              <Input
                size="small"
                value={currentAttributes.iconPadding || ''}
                onChange={(e) => onUpdate({ iconPadding: e.target.value.trim() === '' ? undefined : e.target.value })}
                placeholder="0px"
                style={{ marginTop: '4px' }}
              />
            </div>
          </Col>
          <Col span={12}>
            <div className="mb-2">
              <span className="text-xs text-gray-500">{t`Text padding`}</span>
              <Input
                size="small"
                value={currentAttributes.textPadding || ''}
                onChange={(e) => onUpdate({ textPadding: e.target.value.trim() === '' ? undefined : e.target.value })}
                placeholder="4px 4px 4px 0"
                style={{ marginTop: '4px' }}
              />
            </div>
          </Col>
        </Row>
      </InputLayout>

      <InputLayout label={t`Padding`} layout="vertical">
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
          onChange={(values: { top?: string; right?: string; bottom?: string; left?: string }) => {
            onUpdate({
              paddingTop: values.top,
              paddingRight: values.right,
              paddingBottom: values.bottom,
              paddingLeft: values.left
            })
          }}
        />
      </InputLayout>

      <InputLayout label={t`Icon spacing`} help={t`Padding around each icon`} layout="vertical">
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
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    templateDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSocialAttributes
    // The editor's defaults, then the template's default mj-social attributes
    const blockDefaults = { ...this.getDefaults(), ...templateDefaults } as MJSocialAttributes

    // What an icon shows when neither it nor the block sets an inherited attribute: the template's default
    // mj-social-element attributes (they win over what mj-social passes), then mj-social's, then the element's
    const attributeDefaults = emailTree ? EmailBlockClass.extractAttributeDefaults(emailTree) : {}
    const iconDefaults: Record<string, string | undefined> = {}
    for (const key of SOCIAL_INHERITED_ATTRIBUTES) {
      const value =
        attributeDefaults['mj-social-element']?.[key] ??
        attributeDefaults['mj-social']?.[key] ??
        (MJ_SOCIAL_ELEMENT_DEFAULTS as Record<string, unknown>)[key]
      iconDefaults[key] = typeof value === 'string' ? value : undefined
    }

    const importedFonts: Array<{ name: string; href: string }> = []
    if (emailTree) {
      for (const fontBlock of EmailBlockClass.findAllBlocksByType(emailTree, 'mj-font')) {
        const attrs = fontBlock.attributes as { name?: string; href?: string }
        if (attrs?.name && attrs?.href) importedFonts.push({ name: attrs.name, href: attrs.href })
      }
    }

    return (
      <MjSocialSettingsPanel
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        iconDefaults={iconDefaults}
        importedFonts={importedFonts}
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

    // What the block passes to its elements, as in the email
    const inherited = socialInheritedAttributes(
      this.block.attributes as Record<string, unknown> | undefined,
      attributeDefaults['mj-social']
    )

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
                  {socialElementBlock.getEdit(props, inherited)}
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
                  {socialElementBlock.getEdit(props, inherited)}
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
        data-display-condition={this.block.display_condition ? "true" : undefined}
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
