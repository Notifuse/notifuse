import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Switch } from 'antd'
import type { EmailBlock, MJMLComponentType, MJWrapperAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { SegmentedField } from '../ui/fields/SegmentedField'
import { useAlignOptions } from '../ui/fields/segmentedOptions'
import { CornersField } from '../ui/fields/CornersField'
import BorderField from '../ui/fields/BorderField'
import { BackgroundFields, type BackgroundValues } from '../ui/fields/BackgroundFields'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjWrapperSettingsPanelProps {
  blockId: string
  currentAttributes: MJWrapperAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const WRAPPER_BACKGROUND = [
  'backgroundColor',
  'backgroundUrl',
  'backgroundSize',
  'backgroundRepeat',
  'backgroundPosition',
  'backgroundPositionX',
  'backgroundPositionY'
] as const
const WRAPPER_BORDERS = ['borderTop', 'borderRight', 'borderBottom', 'borderLeft'] as const
const WRAPPER_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjWrapperSettingsPanel: React.FC<MjWrapperSettingsPanelProps> = ({
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
  const background = Object.fromEntries(WRAPPER_BACKGROUND.map((key) => [key, f.value(key)])) as BackgroundValues
  const inheritedBackground = Object.fromEntries(
    WRAPPER_BACKGROUND.map((key) => [key, f.inherited(key)])
  ) as BackgroundValues
  // An empty full-width is the block's own choice (off), so it is read before the inherited one.
  const fullWidth = (own.fullWidth ?? (blockDefaults as Record<string, unknown>).fullWidth) === 'full-width'

  return (
    <>
      <SettingsSection
        sectionKey="mj-wrapper:background"
        title={t`Background`}
        first
        setCount={f.setCount(...WRAPPER_BACKGROUND)}
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
        sectionKey="mj-wrapper:layout"
        title={t`Layout`}
        setCount={f.setCount('fullWidth', 'textAlign')}
        summary={[fullWidth ? t`Full width` : undefined, f.value('textAlign')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Full width`}
          help={t`Makes the wrapper span the entire email viewport width, ignoring container constraints (typically 600px). Useful for full-bleed backgrounds and hero sections.`}
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
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-wrapper:border"
        title={t`Border and corners`}
        setCount={f.setCount(...WRAPPER_BORDERS, 'borderRadius')}
        summary={f.value('borderRadius')}
      >
        <SettingsRow label={t`Border`} vertical isSet={f.isSet(...WRAPPER_BORDERS)} onReset={f.reset(...WRAPPER_BORDERS)}>
          <BorderField
            emailColors={colors}
            value={Object.fromEntries(WRAPPER_BORDERS.map((key) => [key, f.value(key)]))}
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
        sectionKey="mj-wrapper:spacing"
        title={t`Spacing`}
        setCount={f.setCount(...WRAPPER_PADDING)}
        summary={WRAPPER_PADDING.map((key) => f.value(key)).filter(Boolean).join(' · ')}
      >
        <SpacingField
          label={t`Around the sections`}
          centerLabel={t`Sections`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-wrapper" fields={f} />
    </>
  )
}

// Functional component for empty wrapper placeholder with i18n support
const MjWrapperEmptyPlaceholder: React.FC = () => {
  const { t } = useLingui()

  return (
    <div
      style={{
        padding: '20px',
        backgroundColor: '#f8f9fa',
        border: '2px dashed #dee2e6',
        borderRadius: '4px',
        color: '#6c757d',
        fontSize: '14px',
        textAlign: 'center',
        margin: '10px'
      }}
    >
      {t`This wrapper has no sections. Add a section to display content.`}
    </div>
  )
}

/**
 * Implementation for mj-wrapper blocks
 * A wrapper component that can contain sections and provides styling context
 */
export class MjWrapperBlock extends BaseEmailBlock {
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
        <path d="M5 3a2 2 0 0 0-2 2" />
        <path d="M19 3a2 2 0 0 1 2 2" />
        <path d="M21 19a2 2 0 0 1-2 2" />
        <path d="M5 21a2 2 0 0 1-2-2" />
        <path d="M9 3h1" />
        <path d="M9 21h1" />
        <path d="M14 3h1" />
        <path d="M14 21h1" />
        <path d="M3 9v1" />
        <path d="M21 9v1" />
        <path d="M3 14v1" />
        <path d="M21 14v1" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Wrapper'
  }

  getDescription(): React.ReactNode {
    return 'Container that wraps sections and provides styling context'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-wrapper'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-section', 'mj-raw', 'mj-liquid']
  }

  /**
   * Render the settings panel for the wrapper block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjWrapperSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJWrapperAttributes}
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
      attributeDefaults,
      emailTree,
      onUpdateBlock,
      onCloneBlock,
      onDeleteBlock,
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
      'mj-wrapper',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    const wrapperStyle: React.CSSProperties = {
      width: '100%',
      backgroundColor: attrs.fullWidthBackgroundColor,
      backgroundImage: attrs.backgroundUrl ? `url(${attrs.backgroundUrl})` : undefined,
      backgroundRepeat: attrs.backgroundRepeat,
      backgroundSize: attrs.backgroundSize,
      backgroundPosition: attrs.backgroundPosition,
      ...selectionStyle
    }

    const innerWrapperStyle: React.CSSProperties = {
      margin: '0 auto',
      backgroundColor: attrs.backgroundColor,
      paddingTop: attrs.paddingTop,
      paddingRight: attrs.paddingRight,
      paddingBottom: attrs.paddingBottom,
      paddingLeft: attrs.paddingLeft,
      textAlign: attrs.textAlign as React.CSSProperties['textAlign'],
      border: attrs.border !== 'none' ? attrs.border : undefined,
      borderTop: attrs.borderTop !== 'none' ? attrs.borderTop : undefined,
      borderRight: attrs.borderRight !== 'none' ? attrs.borderRight : undefined,
      borderBottom: attrs.borderBottom !== 'none' ? attrs.borderBottom : undefined,
      borderLeft: attrs.borderLeft !== 'none' ? attrs.borderLeft : undefined,
      borderRadius: attrs.borderRadius !== '0px' ? attrs.borderRadius : undefined
    }

    // Check if wrapper has no sections
    const hasSections =
      this.block.children && this.block.children.some((child) => child.type === 'mj-section')

    return (
      <div
        key={key}
        style={{ ...wrapperStyle, position: 'relative' }}
        className={`${attrs.cssClass} ${blockClasses}`.trim()}
        onClick={handleClick}
        data-block-id={this.block.id}
        data-display-condition={this.block.display_condition ? "true" : undefined}
      >
        <div style={innerWrapperStyle}>
          {!hasSections ? (
            <MjWrapperEmptyPlaceholder />
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
          )}
        </div>
      </div>
    )
  }
}
