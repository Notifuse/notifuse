import React from 'react'
import { useLingui } from '@lingui/react/macro'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFont } from '@fortawesome/free-solid-svg-icons'
import type { MJMLComponentType, MJFontAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import ImportFontInput from '../ui/ImportFontInput'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'

// The settings panel of a font import
interface MjFontSettingsPanelProps {
  currentAttributes: MJFontAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjFontSettingsPanel: React.FC<MjFontSettingsPanelProps> = ({
  currentAttributes,
  onUpdate
}) => {
  const { t } = useLingui()

  return (
    <SettingsSection
      sectionKey="mj-font:font"
      title={t`Font`}
      first
      summary={currentAttributes.name}
      help={
        <ul className="m-0 list-inside list-disc p-0">
          <li>
            {t`Import fonts from hosted CSS files (like Google Fonts) to use in your email. The font will only take effect if you actually use it in text elements.`}
          </li>
          <li>{t`Test in different email clients as support varies`}</li>
        </ul>
      }
    >
      <SettingsRow
        label={t`Imported font`}
        vertical
        help={
          <>
            <p className="m-0 mb-1">{t`Configure both the font name and CSS file URL together`}</p>
            <p className="m-0 mb-1">
              {t`Use the font name in text elements:`} <code className="font-mono">font-family="Raleway, Arial"</code>
            </p>
            <p className="m-0">{t`Always provide fallback fonts for better compatibility`}</p>
          </>
        }
      >
        <ImportFontInput
          value={{
            name: currentAttributes.name,
            href: currentAttributes.href
          }}
          onChange={(value) => {
            if (value) {
              onUpdate({
                name: value.name,
                href: value.href
              })
            } else {
              onUpdate({
                name: undefined,
                href: undefined
              })
            }
          }}
          buttonText={t`Import Font`}
        />
      </SettingsRow>
    </SettingsSection>
  )
}

/**
 * Implementation for mj-font blocks (custom font imports)
 */
export class MjFontBlock extends BaseEmailBlock {
  getIcon(): React.ReactNode {
    return <FontAwesomeIcon icon={faFont} className="opacity-70" />
  }

  getLabel(): string {
    return 'Font Import'
  }

  getDescription(): React.ReactNode {
    return 'Import custom fonts from hosted CSS files'
  }

  getCategory(): 'content' | 'layout' {
    return 'layout'
  }

  getDefaults(): Record<string, unknown> {
    return MJML_COMPONENT_DEFAULTS['mj-font'] || {}
  }

  canHaveChildren(): boolean {
    return false
  }

  getValidChildTypes(): MJMLComponentType[] {
    return []
  }

  getEdit(): React.ReactNode {
    // Font blocks don't render in preview (they're configuration)
    return null
  }

  /**
   * Render the settings panel for the font block
   */
  renderSettingsPanel(
    onUpdate: OnUpdateAttributesFunction
  ): React.ReactNode {
    const currentAttributes = this.block.attributes as MJFontAttributes
    return (
      <MjFontSettingsPanel
        currentAttributes={currentAttributes}
        onUpdate={onUpdate}
      />
    )
  }
}
