import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { Alert } from 'antd'
import type { EmailBlock, MJMLComponentType, MJBodyAttributes, MergedBlockAttributes } from '../types'
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
import { AdvancedSection } from './AdvancedSection'
import { panelFields, useEmailColors } from './panelFields'

// Functional component for settings panel with i18n support
interface MjBodySettingsPanelProps {
  blockId: string
  currentAttributes: MJBodyAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
  emailTree?: EmailBlock
}

// The email width in px, from '640px' or '640'; undefined for anything else.
const parseWidthPx = (width?: string): number | undefined => {
  if (!width) return undefined
  const match = width.match(/^(\d+(?:\.\d+)?)px?$/)
  return match ? parseFloat(match[1]) : undefined
}

const MjBodySettingsPanel: React.FC<MjBodySettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate,
  emailTree
}) => {
  const { t } = useLingui()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)
  const colors = useEmailColors(emailTree)

  // The width the email gets: the body's own, else the inherited one.
  const widthValue = parseWidthPx(f.value('width') ?? f.inherited('width'))
  const showWarning = widthValue !== undefined && widthValue > 650

  return (
    <>
      <SettingsSection
        sectionKey="mj-body:layout"
        title={t`Layout`}
        first
        setCount={f.setCount('width', 'backgroundColor')}
        summary={[f.value('width'), f.value('backgroundColor')].filter(Boolean).join(' · ')}
      >
        <SettingsRow
          label={t`Width`}
          help={t`Up to 999 px. Without one, the email is 600 px wide.`}
          isSet={f.isSet('width')}
          onReset={f.reset('width')}
          alert={
            showWarning ? (
              <Alert
                type="warning"
                showIcon
                title={t`Email widths above 650px may not display correctly in some email clients`}
              />
            ) : undefined
          }
        >
          <NumberUnitField
            aria-label={t`Email width`}
            value={f.value('width')}
            inherited={f.inherited('width')}
            units={['px']}
            max={999}
            coalesceBase={f.base('width')}
            onChange={f.write('width')}
          />
        </SettingsRow>
        <SettingsRow label={t`Background`} isSet={f.isSet('backgroundColor')} onReset={f.reset('backgroundColor')}>
          <ColorField
            aria-label={t`Email background`}
            value={f.value('backgroundColor')}
            inherited={f.inherited('backgroundColor')}
            placeholder={t`None`}
            emailColors={colors}
            onChange={f.write('backgroundColor')}
          />
        </SettingsRow>
      </SettingsSection>

      <AdvancedSection type="mj-body" fields={f} />
    </>
  )
}

/**
 * Implementation for mj-body blocks
 */
export class MjBodyBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return null
  }

  getLabel(): string {
    return 'Body'
  }

  getDescription(): React.ReactNode {
    return 'Main container for email content, contains sections'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-body'] || {}
  }

  canHaveChildren(): boolean {
    return true
  }

  getValidChildTypes(): MJMLComponentType[] {
    return ['mj-wrapper', 'mj-section', 'mj-raw', 'mj-liquid']
  }

  /**
   * Render the settings panel for the body block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes,
    emailTree?: EmailBlock
  ): React.ReactNode {
    return (
      <MjBodySettingsPanel
        blockId={this.block.id}
        currentAttributes={this.block.attributes as MJBodyAttributes}
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
      'mj-body',
      this.block.attributes as Record<string, unknown>,
      attributeDefaults
    )

    const bodyStyle: React.CSSProperties = {
      width: attrs.width,
      backgroundColor: attrs.backgroundColor,
      ...selectionStyle
    }

    return (
      <div
        key={key}
        style={bodyStyle}
        className={`${attrs.cssClass} ${blockClasses}`.trim()}
        onClick={handleClick}
      >
        {this.block.children?.map((child) => (
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
        ))}
      </div>
    )
  }
}
