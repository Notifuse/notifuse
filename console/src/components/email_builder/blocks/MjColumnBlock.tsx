import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { EmailBlock, MJMLComponentType, MJColumnAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { groupChildWidth, sectionShare } from '../utils/sectionChildWidth'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { ColorField } from '../ui/fields/ColorField'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useVerticalAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import BorderField from '../ui/fields/BorderField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjColumnSettingsPanelProps {
  blockId: string
  currentAttributes: MJColumnAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
  /** The width the column takes without one, shown as the width's placeholder */
  inheritedWidth?: string
}

const COLUMN_BORDERS = ['borderTop', 'borderRight', 'borderBottom', 'borderLeft'] as const
const COLUMN_INNER_BORDERS = ['innerBorderTop', 'innerBorderRight', 'innerBorderBottom', 'innerBorderLeft'] as const
const COLUMN_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

// The width a column without one takes, as in the email: the width the type's mj-attributes give
// columns, else its equal share of its section or group, else the inherited (MJML default) width.
function columnWidthPlaceholder(
  tree: EmailBlock | undefined,
  block: EmailBlock,
  inherited: string | undefined
): string | undefined {
  if (!tree) return inherited
  const fromAttributes = EmailBlockClass.extractAttributeDefaults(tree)['mj-column']?.width
  if (typeof fromAttributes === 'string' && fromAttributes !== '') return fromAttributes
  const ancestors = EmailBlockClass.getAncestorIds(tree, block.id)
  const parent = EmailBlockClass.findBlockById(tree, ancestors[ancestors.length - 1] ?? '')
  if (parent?.type === 'mj-section') return sectionShare(parent)
  if (parent?.type === 'mj-group') return groupChildWidth({ ...block, attributes: {} } as EmailBlock, parent)
  return inherited
}

const MjColumnSettingsPanel: React.FC<MjColumnSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree,
  inheritedWidth
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)
  const verticalAlignOptions = useVerticalAlignOptions()

  return (
    <>
      <SettingsSection
        sectionKey="mj-column:layout"
        title={t`Layout`}
        first
        setCount={f.setCount('width', 'verticalAlign')}
        summary={[f.value('width'), f.value('verticalAlign')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Width`}
          help={t`In percent or pixels. Empty: an equal share of the section, or of the group.`}
          isSet={f.isSet('width')}
          onReset={f.reset('width')}
        >
          <NumberUnitField
            aria-label={t`Column width`}
            value={f.value('width')}
            inherited={inheritedWidth ?? f.inherited('width')}
            units={['%', 'px']}
            step={{ '%': 1, px: 10 }}
            coalesceBase={f.base('width')}
            onChange={f.write('width')}
          />
        </SettingsRow>
        <SettingsRow label={t`Vertical alignment`} isSet={f.isSet('verticalAlign')} onReset={f.reset('verticalAlign')}>
          <SegmentedField
            aria-label={t`Vertical alignment`}
            value={f.value('verticalAlign')}
            inherited={f.inherited('verticalAlign')}
            options={verticalAlignOptions}
            onChange={f.write('verticalAlign')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-column:colors"
        title={t`Colors`}
        setCount={f.setCount('backgroundColor', 'innerBackgroundColor')}
      >
        <SettingsRow label={t`Background`} isSet={f.isSet('backgroundColor')} onReset={f.reset('backgroundColor')}>
          <ColorField
            aria-label={t`Column background`}
            value={f.value('backgroundColor')}
            inherited={f.inherited('backgroundColor')}
            placeholder={t`None`}
            emailColors={colors}
            onChange={f.write('backgroundColor')}
          />
        </SettingsRow>
        <SettingsRow
          label={t`Inner background`}
          help={t`Requires a padding`}
          isSet={f.isSet('innerBackgroundColor')}
          onReset={f.reset('innerBackgroundColor')}
        >
          <ColorField
            aria-label={t`Inner background`}
            value={f.value('innerBackgroundColor')}
            inherited={f.inherited('innerBackgroundColor')}
            placeholder={t`None`}
            emailColors={colors}
            onChange={f.write('innerBackgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-column:border"
        title={t`Border and corners`}
        setCount={f.setCount(...COLUMN_BORDERS, 'borderRadius', ...COLUMN_INNER_BORDERS, 'innerBorderRadius')}
        summary={[f.value('borderRadius'), f.value('innerBorderRadius')].filter(Boolean).join(' · ')}
      >
        <SettingsRow label={t`Border`} vertical isSet={f.isSet(...COLUMN_BORDERS)} onReset={f.reset(...COLUMN_BORDERS)}>
          <BorderField
            emailColors={colors}
            value={Object.fromEntries(COLUMN_BORDERS.map((key) => [key, f.value(key)]))}
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
        <SettingsRow
          label={t`Inner border`}
          vertical
          isSet={f.isSet(...COLUMN_INNER_BORDERS)}
          onReset={f.reset(...COLUMN_INNER_BORDERS)}
        >
          <BorderField
            emailColors={colors}
            value={{
              borderTop: f.value('innerBorderTop'),
              borderRight: f.value('innerBorderRight'),
              borderBottom: f.value('innerBorderBottom'),
              borderLeft: f.value('innerBorderLeft')
            }}
            onChange={(borders, options) =>
              f.writeMany(
                {
                  innerBorderTop: borders.borderTop,
                  innerBorderRight: borders.borderRight,
                  innerBorderBottom: borders.borderBottom,
                  innerBorderLeft: borders.borderLeft
                },
                options
              )
            }
          />
        </SettingsRow>
        <SettingsRow label={t`Inner corners`} isSet={f.isSet('innerBorderRadius')} onReset={f.reset('innerBorderRadius')}>
          <CornersField
            aria-label={t`Inner corners`}
            value={f.value('innerBorderRadius')}
            inherited={f.inherited('innerBorderRadius')}
            coalesceBase={f.base('innerBorderRadius')}
            onChange={f.write('innerBorderRadius')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-column:spacing"
        title={t`Spacing`}
        setCount={f.setCount(...COLUMN_PADDING)}
        summary={COLUMN_PADDING.map((key) => f.value(key)).filter(Boolean).join(' · ')}
      >
        <SpacingField
          label={t`Inside the column`}
          centerLabel={t`Content`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-column" fields={f} />
    </>
  )
}

// Functional component for empty column placeholder with i18n support
const MjColumnEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
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
      {t`Empty column. Add text, button, or image content.`}
    </div>
  )
}

/**
 * Implementation for mj-column blocks
 */
export class MjColumnBlock extends BaseEmailBlock {
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

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-column'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-text', 'mj-image', 'mj-button', 'mj-divider', 'mj-spacer', 'mj-social', 'mj-raw', 'mj-liquid']
  }

  /**
   * Render the settings panel for the column block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjColumnSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJColumnAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
        inheritedWidth={columnWidthPlaceholder(
          emailTree,
          this.block,
          (blockDefaults as Record<string, unknown>).width as string | undefined
        )}
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
      'mj-column',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Calculate width percentage for responsive classes
    const widthPercent =
      attrs.width && attrs.width.includes('%') ? attrs.width.replace('%', '') : '100'

    // MJML generates a wrapper div with specific classes and styles
    // Width is always 100% here because the parent <td> in MjSectionBlock handles the actual width constraint
    const columnWrapperStyle: React.CSSProperties = {
      fontSize: '0px',
      textAlign: 'left',
      direction: 'ltr',
      display: 'inline-block',
      verticalAlign: attrs.verticalAlign || 'top',
      width: '100%',
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
      <MjColumnEmptyPlaceholder />
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
