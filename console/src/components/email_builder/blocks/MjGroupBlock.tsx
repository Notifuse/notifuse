import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faLock } from '@fortawesome/free-solid-svg-icons'
import { Alert } from 'antd'
import type { EmailBlock, MJMLComponentType, MJGroupAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import {
  findParentSection,
  groupChildWidth,
  parseWidth,
  sectionChildWidth,
  sectionShare
} from '../utils/sectionChildWidth'
import { bodyWidthOf, sectionOverflow } from '../utils/columnWidths'
import { gutterCells, sectionGutterWidth } from '../utils/sectionGutter'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useDirectionOptions, useVerticalAlignOptions } from '../ui/fields/segmentedOptions'
import { BackgroundFields, type BackgroundValues } from '../ui/fields/BackgroundFields'
import { AdvancedSection } from './AdvancedSection'
import { panelFields, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjGroupSettingsPanelProps {
  blockId: string
  currentAttributes: MJGroupAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
  /** Width the group takes without one: its share of the section, when it is in one */
  sectionShareWidth?: string
  /** How far, in percent, the section's columns and groups go past its width; 0 when they fit */
  sectionOverflow?: number
}

const GROUP_BACKGROUND = [
  'backgroundColor',
  'backgroundUrl',
  'backgroundSize',
  'backgroundRepeat',
  'backgroundPosition',
  'backgroundPositionX',
  'backgroundPositionY'
] as const

const MjGroupSettingsPanel: React.FC<MjGroupSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree,
  sectionShareWidth,
  sectionOverflow = 0
}) => {
  const { t, i18n } = useLingui()
  const over = new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 2 }).format(sectionOverflow)
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)
  const verticalAlignOptions = useVerticalAlignOptions()
  const directionOptions = useDirectionOptions()
  const background = Object.fromEntries(GROUP_BACKGROUND.map((key) => [key, f.value(key)])) as BackgroundValues
  const inheritedBackground = Object.fromEntries(
    GROUP_BACKGROUND.map((key) => [key, f.inherited(key)])
  ) as BackgroundValues

  return (
    <>
      <SettingsSection
        sectionKey="mj-group:layout"
        title={t`Layout`}
        first
        setCount={f.setCount('width', 'verticalAlign', 'height', 'direction')}
        summary={[f.value('width'), f.value('verticalAlign'), f.value('direction')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Width`}
          help={t`In percent or pixels. Empty: the group's share of the section.`}
          isSet={f.isSet('width')}
          onReset={f.reset('width')}
          alert={
            sectionOverflow > 0 ? (
              <Alert
                type="warning"
                showIcon
                title={t`The section's columns and groups go ${over}% past its width: in the email, the last ones move to a new line.`}
              />
            ) : undefined
          }
        >
          <NumberUnitField
            aria-label={t`Group width`}
            value={f.value('width')}
            inherited={f.inherited('width') ?? sectionShareWidth}
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
        <SettingsRow label={t`Height`} isSet={f.isSet('height')} onReset={f.reset('height')}>
          <NumberUnitField
            aria-label={t`Group height`}
            mode="height"
            value={f.value('height')}
            inherited={f.inherited('height')}
            placeholder={t`Auto`}
            step={10}
            coalesceBase={f.base('height')}
            onChange={f.write('height')}
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
        sectionKey="mj-group:background"
        title={t`Background`}
        setCount={f.setCount(...GROUP_BACKGROUND)}
        summary={background.backgroundUrl ? t`Image` : background.backgroundColor}
      >
        <BackgroundFields
          value={background}
          inherited={inheritedBackground}
          emailColors={colors}
          onChange={(updates, options) => f.writeMany({ ...updates }, options)}
        />
      </SettingsSection>

      <AdvancedSection type="mj-group" fields={f} />
    </>
  )
}

// Functional component for empty group placeholder with i18n support
const MjGroupEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
    <div
      style={{
        width: '100%',
        height: '40px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: '#722ed1',
        fontSize: '12px',
        fontStyle: 'italic'
      }}
    >
      {t`Group (drag columns here)`}
    </div>
  )
}

/**
 * Implementation for mj-group blocks
 * mj-group allows you to prevent columns from stacking on mobile.
 * Columns inside a group will stay side by side on mobile.
 */
export class MjGroupBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faLock} className="opacity-70" />
  }

  getLabel(): string {
    return 'Group'
  }

  getDescription(): React.ReactNode {
    return (
      <div>
        <div style={{ marginBottom: 8 }}>Prevent columns from stacking on mobile devices</div>
        <div style={{ opacity: 0.7 }}>
          <div
            style={{
              width: 60,
              height: 30,
              border: '2px solid #722ed1',
              borderRadius: 4,
              backgroundColor: '#f9f0ff',
              padding: 4,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}
          >
            <div style={{ display: 'flex', gap: 2 }}>
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#722ed1',
                  borderRadius: 1
                }}
              />
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#d3adf7',
                  borderRadius: 1
                }}
              />
              <div
                style={{
                  width: 8,
                  height: 16,
                  backgroundColor: '#722ed1',
                  borderRadius: 1
                }}
              />
            </div>
            <FontAwesomeIcon
              icon={faLock}
              style={{
                position: 'absolute',
                marginTop: -20,
                marginLeft: 40,
                fontSize: 8,
                color: '#722ed1'
              }}
            />
          </div>
        </div>
      </div>
    )
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-group'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-column']
  }

  /**
   * Render the settings panel for the group block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJGroupAttributes
    const section = emailTree ? findParentSection(emailTree, this.block.id) : null

    return (
      <MjGroupSettingsPanel
        blockId={this.block.id}
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
        emailTree={emailTree}
        sectionShareWidth={section ? sectionShare(section) : undefined}
        sectionOverflow={
          section && emailTree
            ? sectionOverflow(section, EmailBlockClass.extractAttributeDefaults(emailTree), {
                bodyWidth: bodyWidthOf(emailTree)
              })
            : 0
        }
      />
    )
  }

  getEdit(props: PreviewProps): React.ReactNode {
    const {
      selectedBlockId,
      onSelectBlock,
      onUpdateBlock,
      attributeDefaults,
      emailTree,
      onCloneBlock,
      onDeleteBlock,
      onSaveBlock: onSave,
      savedBlocks
    } = props

    const key = this.block.id
    const isSelected = selectedBlockId === this.block.id
    const blockClasses = `email-block-hover ${isSelected ? 'selected' : ''}`.trim()

    const selectionStyle: React.CSSProperties = isSelected ? { zIndex: 10 } : {}

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation()
      if (onSelectBlock) {
        onSelectBlock(this.block.id)
      }
    }

    const attrs = EmailBlockClass.mergeWithAllDefaults(
      'mj-group',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    const groupStyle: React.CSSProperties = {
      display: 'flex',
      flexDirection: attrs.direction === 'rtl' ? 'row-reverse' : 'row',
      // The section cell carries the group's width; the group fills it
      width: '100%',
      verticalAlign: attrs.verticalAlign as React.CSSProperties['verticalAlign'],
      backgroundColor: attrs.backgroundColor,
      alignItems:
        attrs.verticalAlign === 'middle'
          ? 'center'
          : attrs.verticalAlign === 'bottom'
          ? 'flex-end'
          : 'flex-start',
      ...selectionStyle
    }

    // Render child columns
    const children = this.block.children || []

    // The section's gutter reaches the columns of its groups too, measured against the group's width, in the
    // group's direction (see sectionGutter.ts)
    const section = emailTree ? findParentSection(emailTree, this.block.id) : null
    const gutters = (() => {
      if (!section) return children.map(() => null)
      const sectionAttrs = EmailBlockClass.mergeWithAllDefaults(
        'mj-section',
        section.attributes as Record<string, unknown>,
        attributeDefaults
      )
      const sectionWidth = sectionGutterWidth(emailTree as EmailBlock, section, attributeDefaults)
      const width = parseWidth(sectionChildWidth(this.block, section, attributeDefaults))
      const groupWidthPx = !width ? 0 : width.unit === '%' ? (sectionWidth * width.amount) / 100 : width.amount
      return gutterCells(children, sectionAttrs.gutter, groupWidthPx, attrs.direction, (child) =>
        groupChildWidth(child, this.block)
      )
    })()

    return (
      <div
        key={key}
        style={{ ...groupStyle, position: 'relative' }}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
      >
        {children.length === 0 ? (
          <MjGroupEmptyPlaceholder />
        ) : (
          children.map((child, index) => {
            const rendered = EmailBlockClass.renderEmailBlock(
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
            )
            // Each column takes its width in the group, as in the email, rather than an equal part of the row
            const gutter = gutters[index]
            if (gutter) {
              return (
                <div
                  key={child.id}
                  data-group-column-width={gutter.width}
                  style={{
                    flex: `0 1 ${gutter.width}`,
                    maxWidth: gutter.width,
                    minWidth: 0,
                    paddingLeft: gutter.paddingLeft,
                    paddingRight: gutter.paddingRight,
                    boxSizing: 'border-box'
                  }}
                >
                  {rendered}
                </div>
              )
            }
            const width = groupChildWidth(child, this.block)
            return width ? (
              <div
                key={child.id}
                data-group-column-width={width}
                style={{ flex: `0 1 ${width}`, maxWidth: width, minWidth: 0 }}
              >
                {rendered}
              </div>
            ) : (
              <React.Fragment key={child.id}>{rendered}</React.Fragment>
            )
          })
        )}
      </div>
    )
  }
}
