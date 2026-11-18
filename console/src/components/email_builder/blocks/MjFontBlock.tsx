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
import { Select } from 'antd'
import { DraftTextField } from '../ui/fields/DraftTextField'
import { fontStylesheetUrl } from '../../../utils/safeHtml'
import { POPULAR_FONTS, isFontStylesheetAddress } from '../utils/fontImport'
import { SettingsRow } from '../ui/fields/SettingsRow'
import { SettingsSection } from '../ui/fields/SettingsSections'

// Liquid examples go into messages as variables: braces in a message are placeholders
const LIQUID_FONT_URL = '{{ font_url }}'
const LIQUID_VARIABLE = '{{ variable }}'

// The settings panel of a font import
interface MjFontSettingsPanelProps {
  currentAttributes: MJFontAttributes
  onUpdate: OnUpdateAttributesFunction
}

const MjFontSettingsPanel: React.FC<MjFontSettingsPanelProps> = ({ currentAttributes, onUpdate }) => {
  const { t } = useLingui()
  // A font import is used only once it has both: say so while one is missing
  const incomplete = !currentAttributes.name?.trim() || !currentAttributes.href?.trim()
  const popular = POPULAR_FONTS.find(
    (font) => font.name === currentAttributes.name && font.url === currentAttributes.href
  )

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
      <SettingsRow label={t`Google Fonts`} help={t`Fills in the name and the stylesheet of a popular Google Font.`}>
        <Select
          size="small"
          className="w-full"
          aria-label={t`Popular Google Font`}
          placeholder={t`Pick a font`}
          value={popular?.name}
          onChange={(name: string) => {
            const font = POPULAR_FONTS.find((candidate) => candidate.name === name)
            if (font) onUpdate({ name: font.name, href: font.url })
          }}
          options={POPULAR_FONTS.map((font) => ({ value: font.name, label: font.name }))}
        />
      </SettingsRow>
      {incomplete ? (
        <div role="alert" className="mb-1.5 rounded-md bg-warning-bg px-2.5 py-2 text-xs text-warning-fg">
          {t`The font is imported once it has a name and a stylesheet.`}
        </div>
      ) : null}
      <SettingsRow
        label={t`Name`}
        help={
          <>
            <p className="m-0 mb-1">
              {t`Use the font name in text elements:`} <code className="font-mono">font-family="Raleway, Arial"</code>
            </p>
            <p className="m-0">{t`Always provide fallback fonts for better compatibility`}</p>
          </>
        }
      >
        <DraftTextField
          aria-label={t`Font name`}
          value={currentAttributes.name}
          placeholder={t`e.g., Raleway, Open Sans`}
          // A font import needs a name: an emptied field keeps the saved one
          onChange={(name) => name !== undefined && onUpdate({ name })}
        />
      </SettingsRow>
      <SettingsRow label={t`Stylesheet`} help={t`The address of the font's CSS file, or a Liquid output.`}>
        <DraftTextField
          aria-label={t`Stylesheet URL`}
          value={currentAttributes.href}
          placeholder={t`https://fonts.googleapis.com/css?family=… or ${LIQUID_FONT_URL}`}
          mono
          linkOf={fontStylesheetUrl}
          validate={(href) =>
            isFontStylesheetAddress(href)
              ? null
              : t`Invalid URL format. Use a valid URL or a Liquid expression like ${LIQUID_VARIABLE}.`
          }
          // A font import needs a stylesheet: an emptied field keeps the saved one
          onChange={(href) => href !== undefined && onUpdate({ href })}
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
