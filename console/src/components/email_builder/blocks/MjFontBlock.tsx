import React from 'react'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFont } from '@fortawesome/free-solid-svg-icons'
import type { MJMLComponentType, MJFontAttributes } from '../types'
import {
  BaseEmailBlock,
  type OnUpdateAttributesFunction
} from './BaseEmailBlock'
import { MJML_COMPONENT_DEFAULTS } from '../mjml-defaults'
import PanelLayout from '../panels/PanelLayout'
import InputLayout from '../ui/InputLayout'
import ImportFontInput from '../ui/ImportFontInput'
import { faLightbulb } from '@fortawesome/free-regular-svg-icons'
import { Alert } from 'antd'

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
      <PanelLayout title="Font Attributes">
        <Alert
          type="info"
          message={
            <>
              <div className="text-xs text-gray-600">
                <div className="font-medium mb-1">
                  <FontAwesomeIcon icon={faLightbulb} className="mr-2" />
                  How to use:
                </div>
                <ul className="space-y-1 ml-3">
                  <li>
                    • Import fonts from hosted CSS files (like Google Fonts) to use in your email.
                    The font will only take effect if you actually use it in text elements.
                  </li>
                  <li>
                    • Use the font name in text elements:{' '}
                    <code className="bg-white px-1 rounded">font-family="Raleway, Arial"</code>
                  </li>
                  <li>• Always provide fallback fonts for better compatibility</li>
                  <li>• Test in different email clients as support varies</li>
                </ul>
              </div>
            </>
          }
        />

        <InputLayout
          label="Font Configuration"
          help="Configure both the font name and CSS file URL together"
          layout="vertical"
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
            buttonText="Import Font"
          />
        </InputLayout>
      </PanelLayout>
    )
  }
}
