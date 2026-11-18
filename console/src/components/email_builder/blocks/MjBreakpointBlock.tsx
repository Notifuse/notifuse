import React from 'react'
import { useLingui } from '@lingui/react/macro'
import type { MJMLComponentType, MJBreakpointAttributes, MergedBlockAttributes } from '../types'
import { BaseEmailBlock, type OnUpdateAttributesFunction } from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import { faMobileAlt } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'
import { NumberUnitField } from '../ui/fields/NumberUnitField'
import { panelFields } from './panelFields'
import { Button } from 'antd'
import { useEditorActions } from '../ui/fields/editorActions'

// The settings panel of the mobile breakpoint
interface MjBreakpointSettingsPanelProps {
  blockId: string
  currentAttributes: MJBreakpointAttributes
  blockDefaults: MergedBlockAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjBreakpointSettingsPanel: React.FC<MjBreakpointSettingsPanelProps> = ({
  blockId,
  currentAttributes,
  blockDefaults,
  onUpdate
}) => {
  const { t } = useLingui()
  const { showPreview } = useEditorActions()
  const f = panelFields(blockId, currentAttributes as Record<string, unknown>, blockDefaults as Record<string, unknown>, onUpdate)

  return (
    <SettingsSection sectionKey="mj-breakpoint:mobile" title={t`Mobile`} first setCount={f.setCount('width')}>
      <SettingsRow
        label={t`Breakpoint width`}
        isSet={f.isSet('width')}
        onReset={f.reset('width')}
        help={
          <>
            <p className="m-0 mb-1">
              {t`Screen width below which mobile layout is activated. Common values: 480px, 600px, 768px`}
            </p>
            <p className="m-0 mb-1">
              {t`Sets the screen width below which the email will switch to mobile layout. Most email clients use 480px as the standard mobile breakpoint.`}
            </p>
            <div className="mb-1 font-semibold">{t`How it works:`}</div>
            <ul className="m-0 mb-1 list-inside list-disc p-0">
              <li>{t`Sets the responsive breakpoint for the entire email`}</li>
              <li>{t`Only one breakpoint per email template is allowed`}</li>
              <li>{t`Affects how columns stack on mobile devices`}</li>
              <li>{t`Standard value is 480px for most email clients`}</li>
            </ul>
            <div className="mb-1 font-semibold">{t`Preview mode required`}</div>
            <p className="m-0">
              {t`Switch to Preview mode to see the breakpoint effects. The edit mode shows the raw structure, but only preview mode compiles the MJML template and renders the responsive behavior properly.`}
            </p>
          </>
        }
      >
        <NumberUnitField
          aria-label={t`Breakpoint width`}
          value={f.value('width')}
          inherited={f.inherited('width')}
          placeholder="480"
          units={['px']}
          min={240}
          max={1200}
          step={10}
          coalesceBase={f.base('width')}
          onChange={f.write('width')}
        />
      </SettingsRow>
      {showPreview ? (
        <div className="ml-26">
          <Button type="link" size="small" onClick={showPreview}>
            {t`See in preview`}
          </Button>
        </div>
      ) : null}
    </SettingsSection>
  )
}

/**
 * Implementation for mj-breakpoint blocks (responsive breakpoint configuration)
 */
export class MjBreakpointBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faMobileAlt} className="opacity-70" />
  }

  getLabel(): string {
    return 'Breakpoint'
  }

  getDescription(): React.ReactNode {
    return 'Configure the responsive breakpoint for mobile view'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-breakpoint'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Breakpoint blocks don't render in preview (they're configuration)
    return null
  }

  /**
   * Render the settings panel for the breakpoint block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction,
    blockDefaults: MergedBlockAttributes
  ): React.ReactNode {
    const currentAttributes = (this.block.attributes ?? {}) as MJBreakpointAttributes
    return (
      <MjBreakpointSettingsPanel
        blockId={this.block.id}
        currentAttributes={currentAttributes}
        blockDefaults={blockDefaults}
        onUpdate={onUpdate}
      />
    )
  }
}
