import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Switch } from 'antd'
import type { EmailBlock, MJMLComponentType, MJSectionAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { sectionChildWidth } from '../utils/sectionChildWidth'
import { gutterCells, sectionGutterWidth } from '../utils/sectionGutter'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions, useDirectionOptions } from '../ui/fields/segmentedOptions'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { CornersField } from '../ui/fields/CornersField'
import BorderField from '../ui/fields/BorderField'
import { BackgroundFields, type BackgroundValues } from '../ui/fields/BackgroundFields'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjSectionSettingsPanelProps {
  blockId: string
  currentAttributes: MJSectionAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const SECTION_BACKGROUND = [
  'backgroundColor',
  'backgroundUrl',
  'backgroundSize',
  'backgroundRepeat',
  'backgroundPosition',
  'backgroundPositionX',
  'backgroundPositionY'
] as const
const SECTION_BORDERS = ['borderTop', 'borderRight', 'borderBottom', 'borderLeft'] as const
const SECTION_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjSectionSettingsPanel: React.FC<MjSectionSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const own = currentAttributes as Record<string, unknown>
  const f = panelFields(blockId, own, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)
  const alignOptions = useAlignOptions()
  const directionOptions = useDirectionOptions()
  const background = Object.fromEntries(SECTION_BACKGROUND.map((key) => [key, f.value(key)])) as BackgroundValues
  const inheritedBackground = Object.fromEntries(
    SECTION_BACKGROUND.map((key) => [key, f.inherited(key)])
  ) as BackgroundValues
  // An empty full-width is the block's own choice (off), so it is read before the inherited one.
  const fullWidth = (own.fullWidth ?? (blockDefaults as Record<string, unknown>).fullWidth) === 'full-width'
  const gutter = f.value('gutter')

  return (
    <>
      <SettingsSection
        sectionKey="mj-section:background"
        title={t`Background`}
        first
        setCount={f.setCount(...SECTION_BACKGROUND)}
        summary={background.backgroundUrl ? t`Image` : background.backgroundColor}
      >
        <BackgroundFields
          value={background}
          inherited={inheritedBackground}
          emailColors={colors}
          onChange={(updates, options) => f.writeMany({ ...updates }, options)}
        />
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-section:layout"
        title={t`Layout`}
        setCount={f.setCount('fullWidth', 'textAlign', 'direction')}
        summary={[fullWidth ? t`Full width` : undefined, f.value('textAlign'), f.value('direction')]
          .filter(Boolean)
          .join(' · ')}
      >
        <SettingsRow
          label={t`Full width`}
          help={t`Makes the section span the entire email viewport width, ignoring container constraints (typically 600px). Useful for full-bleed backgrounds and hero sections.`}
          isSet={f.isSet('fullWidth')}
          onReset={f.reset('fullWidth')}
        >
          <Switch
            size="small"
            aria-label={t`Full width`}
            checked={fullWidth}
            onChange={(checked) => f.write('fullWidth')(checked ? 'full-width' : '')}
          />
        </SettingsRow>
        <SettingsRow label={t`Text alignment`} isSet={f.isSet('textAlign')} onReset={f.reset('textAlign')}>
          <SegmentedField
            aria-label={t`Text alignment`}
            value={f.value('textAlign')}
            inherited={f.inherited('textAlign')}
            options={alignOptions}
            onChange={f.write('textAlign')}
          />
        </SettingsRow>
        <SettingsRow label={t`Direction`} isSet={f.isSet('direction')} onReset={f.reset('direction')}>
          <SegmentedField
            aria-label={t`Direction`}
            value={f.value('direction')}
            inherited={f.inherited('direction')}
            options={directionOptions}
            onChange={f.write('direction')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-section:border"
        title={t`Border and corners`}
        setCount={f.setCount(...SECTION_BORDERS, 'borderRadius')}
        summary={f.value('borderRadius')}
      >
        <SettingsRow label={t`Border`} vertical isSet={f.isSet(...SECTION_BORDERS)} onReset={f.reset(...SECTION_BORDERS)}>
          <BorderField
            emailColors={colors}
            value={Object.fromEntries(SECTION_BORDERS.map((key) => [key, f.value(key)]))}
            onChange={(borders, options) => f.writeMany({ ...borders }, options)}
          />
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
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-section:spacing"
        title={t`Spacing`}
        setCount={f.setCount(...SECTION_PADDING, 'gutter')}
        summary={[...SECTION_PADDING.map((key) => f.value(key)), gutter ? t`gap ${gutter}` : undefined]
          .filter(Boolean)
          .join(' · ')}
      >
        <SpacingField
          label={t`Around the columns`}
          centerLabel={t`Columns`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
        <div className="mt-2">
          <SettingsRow
            label={t`Column gap`}
            help={t`Space between the columns, in pixels or percent. The outer edges of the section get none.`}
            isSet={f.isSet('gutter')}
            onReset={f.reset('gutter')}
          >
            <NumberUnitField
              aria-label={t`Column gap`}
              value={gutter}
              inherited={f.inherited('gutter')}
              placeholder={t`None`}
              units={['px', '%']}
              defaultUnit="px"
              step={{ '%': 1, px: 10 }}
              coalesceBase={f.base('gutter')}
              onChange={f.write('gutter')}
            />
          </SettingsRow>
        </div>
      </SettingsSection>

      <AdvancedSection type="mj-section" fields={f} />
    </>
  )
}

// Functional component for empty section placeholder with i18n support
const MjSectionEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
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
      {t`This section is empty. Add columns or groups to display content.`}
    </div>
  )
}

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

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-section'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-column', 'mj-group', 'mj-raw', 'mj-liquid']
  }

  /**
   * Render the settings panel for the section block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjSectionSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJSectionAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
      />
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
      this.block.attributes as Record<string, unknown>,
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
      borderRadius: attrs.borderRadius !== '0px' ? attrs.borderRadius : undefined,
      overflow: attrs.borderRadius !== '0px' ? 'hidden' : undefined,
      ...selectionStyle
    }

    // MJML table style - duplicates background for email client compatibility
    const tableStyle: React.CSSProperties = {
      backgroundColor: attrs.backgroundColor,
      width: '100%',
      borderRadius: attrs.borderRadius !== '0px' ? attrs.borderRadius : undefined,
      borderTop: attrs.borderTop !== 'none' ? attrs.borderTop : undefined,
      borderRight: attrs.borderRight !== 'none' ? attrs.borderRight : undefined,
      borderBottom: attrs.borderBottom !== 'none' ? attrs.borderBottom : undefined,
      borderLeft: attrs.borderLeft !== 'none' ? attrs.borderLeft : undefined
    }

    // MJML td style - handles padding, direction, text-align
    const cellStyle: React.CSSProperties = {
      direction: (attrs.direction as 'ltr' | 'rtl') || 'ltr',
      fontSize: '0px', // MJML sets this to 0 to prevent spacing issues
      padding: `${attrs.paddingTop || '20px'} ${attrs.paddingRight || '0'} ${
        attrs.paddingBottom || '20px'
      } ${attrs.paddingLeft || '0'}`,
      textAlign: (attrs.textAlign as 'left' | 'center' | 'right' | 'justify') || 'left'
    }

    // The gutter MJML 5 puts between the columns: each column cell gets a narrower width and padding on the sides
    // that face another child (see sectionGutter.ts)
    const children = this.block.children ?? []
    const gutters = gutterCells(
      children,
      attrs.gutter,
      sectionGutterWidth(emailTree, this.block, attributeDefaults),
      attrs.direction,
      (child) => sectionChildWidth(child, this.block, attributeDefaults)
    )

    // Check if section has no columns or groups
    const hasContent =
      this.block.children &&
      this.block.children.some((child) => child.type === 'mj-column' || child.type === 'mj-group')

    const contentElement = !hasContent ? (
      <MjSectionEmptyPlaceholder />
    ) : (
      // Wrap columns in a table row structure as MJML does for multiple columns
      (() => {
        return (
          <table
            role="presentation"
            border={0}
            cellPadding="0"
            cellSpacing="0"
            style={{ width: '100%' }}
          >
            <tbody>
              <tr>
                {children.map((child, index) => {
                  // Columns and groups carry their width on their cell, as in the email
                  const columnWidth = sectionChildWidth(child, this.block, attributeDefaults)
                  const gutter = gutters[index]

                  return (
                    <td
                      key={child.id}
                      style={
                        gutter
                          ? {
                              verticalAlign: 'top',
                              width: gutter.width,
                              paddingLeft: gutter.paddingLeft,
                              paddingRight: gutter.paddingRight,
                              boxSizing: 'border-box'
                            }
                          : {
                              verticalAlign: 'top',
                              width: columnWidth
                            }
                      }
                    >
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
                  )
                })}
              </tr>
            </tbody>
          </table>
        )
      })()
    )

    // Simulate MJML's exact structure: wrapper div > table > tbody > tr > td > columns
    return (
      <div
        key={key}
        style={{ ...wrapperStyle, position: 'relative' }}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
        data-display-condition={this.block.display_condition ? "true" : undefined}
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
