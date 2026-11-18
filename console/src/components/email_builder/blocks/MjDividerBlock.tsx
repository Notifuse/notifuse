import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Select } from 'antd'
import type { MJMLComponentType, EmailBlock, MJDividerAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction, type PreviewProps } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { ColorField } from '../ui/fields/ColorField'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions } from '../ui/fields/segmentedOptions'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjDividerSettingsPanelProps {
  blockId: string
  currentAttributes: MJDividerAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const DIVIDER_LINE = ['borderColor', 'borderWidth', 'borderStyle'] as const
const DIVIDER_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjDividerSettingsPanel: React.FC<MjDividerSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(
    blockId,
    currentAttributes as Record<string, unknown>,
    blockDefaults as Record<string, unknown>,
    onUpdate
  )
  const colors = useEmailColors(emailTree)
  const alignOptions = useAlignOptions()
  // The three styles mj-divider accepts (border-style of its line)
  const styleOptions = [
    { value: 'solid', label: t`Solid` },
    { value: 'dashed', label: t`Dashed` },
    { value: 'dotted', label: t`Dotted` }
  ]
  const inheritedStyle = f.inherited('borderStyle') ?? 'solid'

  return (
    <>
      <SettingsSection
        sectionKey="mj-divider:line"
        title={t`Line`}
        first
        setCount={f.setCount(...DIVIDER_LINE, 'width', 'align')}
        summary={[f.value('borderWidth'), f.value('borderStyle'), f.value('width')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Line`}
          help={t`Color, thickness from 0 to 50 px (4 by default), and style: solid, dashed or dotted.`}
          isSet={f.isSet(...DIVIDER_LINE)}
          onReset={f.reset(...DIVIDER_LINE)}
        >
          <div className="grid grid-cols-2 items-center gap-1.5">
            <div className="col-span-2">
              <ColorField
                aria-label={t`Line color`}
                value={f.value('borderColor')}
                inherited={f.inherited('borderColor')}
                emailColors={colors}
                onChange={f.write('borderColor')}
              />
            </div>
            <NumberUnitField
              aria-label={t`Line thickness`}
              value={f.value('borderWidth')}
              inherited={f.inherited('borderWidth')}
              placeholder="4"
              units={['px']}
              min={0}
              max={50}
              coalesceBase={f.base('borderWidth')}
              onChange={f.write('borderWidth')}
            />
            <Select
              size="small"
              className="w-full"
              aria-label={t`Line style`}
              value={f.value('borderStyle')}
              placeholder={styleOptions.find((option) => option.value === inheritedStyle)?.label ?? inheritedStyle}
              options={styleOptions}
              onChange={(value) => f.write('borderStyle')(value)}
            />
          </div>
        </SettingsRow>
        <SettingsRow label={t`Width`} isSet={f.isSet('width')} onReset={f.reset('width')}>
          <NumberUnitField
            aria-label={t`Divider width`}
            value={f.value('width')}
            inherited={f.inherited('width')}
            placeholder={t`Auto`}
            units={['%', 'px']}
            defaultUnit="%"
            step={{ '%': 1, px: 10 }}
            coalesceBase={f.base('width')}
            onChange={f.write('width')}
          />
        </SettingsRow>
        <SettingsRow label={t`Alignment`} isSet={f.isSet('align')} onReset={f.reset('align')}>
          <SegmentedField
            aria-label={t`Divider alignment`}
            value={f.value('align')}
            inherited={f.inherited('align')}
            options={alignOptions}
            onChange={f.write('align')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-divider:colors"
        title={t`Colors`}
        setCount={f.setCount('containerBackgroundColor')}
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
            placeholder={t`Transparent`}
            emailColors={colors}
            onChange={f.write('containerBackgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection sectionKey="mj-divider:spacing" title={t`Spacing`} setCount={f.setCount(...DIVIDER_PADDING)}>
        <SpacingField
          label={t`Around the line`}
          centerLabel="———"
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-divider" fields={f} />
    </>
  )
}

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

  getDefaults(): Record<string, unknown> {
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
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjDividerSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJDividerAttributes}
        blockDefaults={blockDefaults}
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

    const selectionStyle: React.CSSProperties = isSelected ? { position: 'relative', zIndex: 10 } : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-divider',
      this.block.attributes as Record<string, unknown> | undefined,
      attributeDefaults
    )

    // Container style (equivalent to the td wrapper in MJML)
    const containerStyle: React.CSSProperties = {
      padding: `${attrs.paddingTop || '10px'} ${attrs.paddingRight || '25px'} ${
        attrs.paddingBottom || '10px'
      } ${attrs.paddingLeft || '25px'}`,
      backgroundColor: attrs.containerBackgroundColor,
      textAlign: (attrs.align as React.CSSProperties['textAlign']) || 'center',
      fontSize: '0px', // MJML sets font-size to 0 on container
      ...selectionStyle
    }

    // Divider line style
    const dividerStyle: React.CSSProperties = {
      display: 'inline-block',
      width: attrs.width || '100%',
      height: '0px',
      border: 'none',
      borderTop: `${attrs.borderWidth || '4px'} ${attrs.borderStyle || 'solid'} ${attrs.borderColor || '#000000'}`,
      margin: '0px',
      fontSize: '1px', // Prevent collapsing in some email clients
      lineHeight: '1px'
    }

    return (
      <div
        key={key}
        style={containerStyle}
        className={blockClasses}
        onClick={handleClick}
        data-block-id={this.block.id}
        data-display-condition={this.block.display_condition ? 'true' : undefined}
      >
        <div style={dividerStyle}>&nbsp;</div>
      </div>
    )
  }
}
