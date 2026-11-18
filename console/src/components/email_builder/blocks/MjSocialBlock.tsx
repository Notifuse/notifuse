import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, MJMLComponentType, MJSocialAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS, MJ_SOCIAL_ELEMENT_DEFAULTS, SOCIAL_INHERITED_ATTRIBUTES } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { MjSocialElementBlock } from './MjSocialElementBlock'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faInstagram } from '@fortawesome/free-brands-svg-icons'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { DraftTextField } from '../ui/fields/DraftTextField'
import { ColorField } from '../ui/fields/ColorField'
import { FontFields } from '../ui/fields/FontFields'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import { SingleBorderField } from '../ui/fields/BorderField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { useImportedFonts, inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// A px length as a number, for the number fields
const pixels = (value?: string): number | undefined => {
  const match = value?.match(/^(\d+(?:\.\d+)?)px$/)
  return match ? parseFloat(match[1]) : undefined
}

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

// The settings panel of an mj-social block
interface MjSocialSettingsPanelProps {
  blockId: string
  currentAttributes: MJSocialAttributes
  /** What the block inherits, and, for what it passes to its icons, what an icon shows without it */
  inherited: Record<string, unknown>
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

// What mj-social carries of the font group: no text-transform, letter-spacing, nor a text align
// of its own (its align places the icons, in the Layout section)
const SOCIAL_FONT_FIELDS = ['fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'textDecoration', 'lineHeight'] as const
const SOCIAL_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjSocialSettingsPanel: React.FC<MjSocialSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  inherited,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, inherited, onUpdate)
  const colors = useEmailColors(emailTree)
  const importedFonts = useImportedFonts(emailTree)
  const alignOptions = useAlignOptions()
  // The layout the block takes without a mode of its own: the template's default mj-social mode, else a row
  const defaultMode = f.inherited('mode') === 'vertical' ? 'vertical' : 'horizontal'
  const fontValues = Object.fromEntries(SOCIAL_FONT_FIELDS.map((key) => [key, f.value(key)]))
  const fontInherited = Object.fromEntries(SOCIAL_FONT_FIELDS.map((key) => [key, f.inherited(key)]))
  const fontSummary = [f.value('fontFamily')?.split(',')[0], f.value('fontSize'), f.value('fontWeight')]
    .filter(Boolean)
    .join(' · ')
  // Without a height, the icons are as high as their size
  const iconHeightPlaceholder = String(pixels(f.value('iconSize')) ?? pixels(f.inherited('iconSize')) ?? 20)

  return (
    <>
      <SettingsSection
        sectionKey="mj-social:layout"
        title={t`Layout`}
        first
        setCount={f.setCount('mode', 'align')}
      >
        <SettingsRow label={t`Layout`} isSet={f.isSet('mode')} onReset={f.reset('mode')}>
          <SegmentedField
            aria-label={t`Layout`}
            value={f.value('mode')}
            inherited={defaultMode}
            options={[
              { label: t`In a row`, value: 'horizontal' },
              { label: t`In a column`, value: 'vertical' }
            ]}
            // The default layout is written as no mode
            onChange={(value) => f.write('mode')(value === defaultMode ? undefined : value)}
          />
        </SettingsRow>
        <SettingsRow label={t`Alignment`} isSet={f.isSet('align')} onReset={f.reset('align')}>
          <SegmentedField
            aria-label={t`Alignment`}
            value={f.value('align')}
            inherited={f.inherited('align')}
            options={alignOptions}
            onChange={f.write('align')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social:font"
        title={t`Font`}
        help={t`Each icon's text takes these, unless the icon sets its own.`}
        setCount={f.setCount(...SOCIAL_FONT_FIELDS)}
        summary={fontSummary}
      >
        <FontFields
          value={fontValues}
          inherited={fontInherited}
          importedFonts={importedFonts}
          fields={SOCIAL_FONT_FIELDS}
          onChange={(updates, options) => f.writeMany(updates, options)}
        />
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social:icons"
        title={t`Icons`}
        help={t`Applies to every icon; an icon's own settings win.`}
        setCount={f.setCount('iconSize', 'iconHeight', 'borderRadius', 'color', 'iconPadding', 'textPadding')}
        summary={f.value('iconSize')}
      >
        <SettingsRow
          label={t`Size, height`}
          help={t`10 to 100 px. Without a height, the icons are as high as their size.`}
          isSet={f.isSet('iconSize', 'iconHeight')}
          onReset={f.reset('iconSize', 'iconHeight')}
        >
          <div className="grid grid-cols-2 gap-1.5">
            <NumberUnitField
              aria-label={t`Icon size`}
              value={f.value('iconSize')}
              inherited={f.inherited('iconSize')}
              placeholder="20"
              units={['px']}
              min={10}
              max={100}
              coalesceBase={f.base('iconSize')}
              onChange={f.write('iconSize')}
            />
            <NumberUnitField
              aria-label={t`Icon height`}
              value={f.value('iconHeight')}
              inherited={f.inherited('iconHeight')}
              placeholder={iconHeightPlaceholder}
              units={['px']}
              min={10}
              max={100}
              coalesceBase={f.base('iconHeight')}
              onChange={f.write('iconHeight')}
            />
          </div>
        </SettingsRow>
        <SettingsRow label={t`Corners`} isSet={f.isSet('borderRadius')} onReset={f.reset('borderRadius')}>
          <CornersField
            aria-label={t`Corners`}
            value={f.value('borderRadius')}
            inherited={f.inherited('borderRadius')}
            coalesceBase={f.base('borderRadius')}
            onChange={f.write('borderRadius')}
          />
        </SettingsRow>
        <SettingsRow label={t`Text color`} isSet={f.isSet('color')} onReset={f.reset('color')}>
          <ColorField
            aria-label={t`Text color`}
            value={f.value('color')}
            inherited={f.inherited('color')}
            emailColors={colors}
            onChange={f.write('color')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Icon padding`}
          help={t`Around each icon's image, as a CSS padding (e.g. 4px 8px).`}
          isSet={f.isSet('iconPadding')}
          onReset={f.reset('iconPadding')}
        >
          <DraftTextField
            aria-label={t`Icon padding`}
            value={f.value('iconPadding')}
            onChange={f.write('iconPadding')}
            placeholder={f.inherited('iconPadding') ?? '0px'}
            mono
          />
        </SettingsRow>
        <SettingsRow
          label={t`Text padding`}
          help={t`Around each icon's text, as a CSS padding (e.g. 4px 4px 4px 0).`}
          isSet={f.isSet('textPadding')}
          onReset={f.reset('textPadding')}
        >
          <DraftTextField
            aria-label={t`Text padding`}
            value={f.value('textPadding')}
            onChange={f.write('textPadding')}
            placeholder={f.inherited('textPadding') ?? '4px 4px 4px 0'}
            mono
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social:colors"
        title={t`Colors`}
        setCount={f.setCount('containerBackgroundColor', 'border')}
      >
        <SettingsRow
          label={t`Container background`}
          isSet={f.isSet('containerBackgroundColor')}
          onReset={f.reset('containerBackgroundColor')}
        >
          <ColorField
            aria-label={t`Container background`}
            value={f.value('containerBackgroundColor')}
            inherited={f.inherited('containerBackgroundColor')}
            emailColors={colors}
            onChange={f.write('containerBackgroundColor')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Icon border`}
          help={t`Applies to every icon; an icon's own border wins.`}
          vertical
          isSet={f.isSet('border')}
          onReset={f.reset('border')}
        >
          <SingleBorderField value={f.value('border')} onChange={f.write('border')} emailColors={colors} />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-social:spacing"
        title={t`Spacing`}
        help={t`Around the block: the block's padding. Icon spacing: the padding around each icon.`}
        setCount={f.setCount(...SOCIAL_PADDING, 'innerPadding')}
      >
        <SpacingField
          label={t`Around the block`}
          centerLabel={t`Icons`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
        <div className="mt-2">
          <SpacingField
            label={t`Icon spacing`}
            centerLabel={t`Icon`}
            value={f.value('innerPadding')}
            inherited={f.inherited('innerPadding') ?? '4px'}
            onChange={(value: string | undefined, options?: FieldWriteOptions) =>
              f.writeMany({ innerPadding: value }, options)
            }
          />
        </div>
      </SettingsSection>

      <AdvancedSection type="mj-social" fields={f} />
    </>
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
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJSocialAttributes
    const inherited: Record<string, unknown> = { ...(blockDefaults as Record<string, unknown>) }

    // For what the block passes to its icons and does not hold itself (font, text color): what an
    // icon shows when neither it nor the block sets it — the template's default mj-social-element
    // attributes (they win over what mj-social passes), then mj-social's, then the element's
    const attributeDefaults = emailTree ? EmailBlockClass.extractAttributeDefaults(emailTree) : {}
    // (not the editor's mj-social defaults: the canvas leaves those keys to the elements' own)
    for (const key of SOCIAL_INHERITED_ATTRIBUTES) {
      const value =
        attributeDefaults['mj-social-element']?.[key] ??
        attributeDefaults['mj-social']?.[key] ??
        (MJ_SOCIAL_ELEMENT_DEFAULTS as Record<string, unknown>)[key]
      if (typeof value === 'string') inherited[key] = value
    }

    return (
      <MjSocialSettingsPanel
        blockId={this.block.id}
        currentAttributes={currentAttributes}
        inherited={inherited}
        onUpdate={onUpdate}
        emailTree={emailTree}
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
