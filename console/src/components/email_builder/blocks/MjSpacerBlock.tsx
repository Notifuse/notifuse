import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, EmailBlock, MJSpacerAttributes, MergedBlockAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction,
  type PreviewProps
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { EmailBlockClass } from '../EmailBlockClass'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { ColorField } from '../ui/fields/ColorField'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { SpacingField } from '../ui/fields/SpacingField'
import { toSideAttributes, type SpacingSides } from '../ui/fields/spacing'
import type { FieldWriteOptions } from '../ui/fields/types'
import { AdvancedSection } from './AdvancedSection'
import { inheritedSidesOf, panelFields, sidesOf, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjSpacerSettingsPanelProps {
  blockId: string
  currentAttributes: MJSpacerAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

const SPACER_PADDING = ['paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft'] as const

const MjSpacerSettingsPanel: React.FC<MjSpacerSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)

  return (
    <>
      <SettingsSection
        sectionKey="mj-spacer:size"
        title={t`Size`}
        first
        setCount={f.setCount('height')}
        summary={f.value('height')}
      >
        <SettingsRow
          label={t`Height`}
          help={t`From 0 to 500 px; 20 px by default.`}
          isSet={f.isSet('height')}
          onReset={f.reset('height')}
        >
          <NumberUnitField
            aria-label={t`Spacer height`}
            value={f.value('height')}
            inherited={f.inherited('height')}
            placeholder="20"
            units={['px']}
            min={0}
            max={500}
            coalesceBase={f.base('height')}
            onChange={f.write('height')}
          />
        </SettingsRow>
      </SettingsSection>

      <SettingsSection
        sectionKey="mj-spacer:colors"
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

      <SettingsSection sectionKey="mj-spacer:spacing" title={t`Spacing`} setCount={f.setCount(...SPACER_PADDING)}>
        <SpacingField
          label={t`Around the spacer`}
          centerLabel={`↕ ${f.value('height') ?? f.inherited('height') ?? '20px'}`}
          value={sidesOf(f, 'padding')}
          inherited={inheritedSidesOf(f, 'padding')}
          onChange={(sides: Partial<SpacingSides>, options?: FieldWriteOptions) =>
            f.writeMany(toSideAttributes(sides, 'padding'), options)
          }
        />
      </SettingsSection>

      <AdvancedSection type="mj-spacer" fields={f} />
    </>
  )
}

/**
 * Implementation for mj-spacer blocks
 */
export class MjSpacerBlock extends BaseEmailBlock {
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
        <path d="M12 22v-6" />
        <path d="M12 8V2" />
        <path d="M4 12H2" />
        <path d="M10 12H8" />
        <path d="M16 12h-2" />
        <path d="M22 12h-2" />
        <path d="m15 19-3 3-3-3" />
        <path d="m15 5-3-3-3 3" />
      </svg>
    )
  }

  getLabel(): string {
    return 'Spacer'
  }

  getDescription(): React.ReactNode {
    return 'Vertical spacing component to add space between content elements'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-spacer'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  /**
   * Render the settings panel for the spacer block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjSpacerSettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJSpacerAttributes}
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
      attributeDefaults
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
      'mj-spacer',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    // Container style (equivalent to the td wrapper in MJML)
    const containerStyle: React.CSSProperties = {
      padding: `${attrs.paddingTop || '0px'} ${attrs.paddingRight || '0px'} ${
        attrs.paddingBottom || '0px'
      } ${attrs.paddingLeft || '0px'}`,
      backgroundColor: attrs.containerBackgroundColor,
      fontSize: '0px', // MJML sets font-size to 0 on container
      lineHeight: '0px',
      ...selectionStyle
    }

    // Spacer content style - creates the vertical space
    const spacerStyle: React.CSSProperties = {
      display: 'block',
      width: '100%',
      height: attrs.height || '20px',
      fontSize: '1px', // Prevent collapsing in some email clients
      lineHeight: '1px',
      backgroundColor: 'transparent' // Spacer itself should be invisible
    }

    return (
      <div
        key={key}
        className={`${attrs.cssClass || ''} ${blockClasses}`.trim()}
        onClick={handleClick}
        style={{ ...containerStyle, position: 'relative' }}
        data-block-id={this.block.id}
        data-display-condition={this.block.display_condition ? "true" : undefined}
      >
        <div style={spacerStyle}>&nbsp;</div>
      </div>
    )
  }
}
